// Package cunion generates C17 facades for explicit member views of union values.
// The native compiler supplies union layout and the function calling convention.
// A selected member is a caller contract, never inferred from object bytes.
package cunion

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// Value selects a scalar, native-address or named struct transport.
// Union optionally names a C union tag ("union Number") or typedef, and Member selects its active member.
// Input unions are zero-initialized before assigning Member. Result Member must
// be valid for the native result. Pointer addresses and pointees remain borrowed.
// Record names an ordinary C struct or typedef when Type is abi.Struct.
// Void is allowed only for an ordinary result. Array views, bitfields, variadics
// and other calling conventions require a different compiled adapter.
type Value struct {
	Type          abi.Type
	Union, Member string
	Record        string
}

// Function selects an exact native C prototype and a distinct exported facade.
// Name and Symbol are C identifiers. Args and Result describe C transports
// and the explicitly selected union members used at the native call boundary.
type Function struct {
	Name, Symbol string
	Args         []Value
	Result       Value
}

// Facade contains independent C17 source/header buffers and ordered export names.
// Compile CSource against the original header with the library's target/flags.
// Parse CHeader with compiler/clang to generate dynamic, cgo or direct llgo calls.
// Struct transports include the original header so the compiler can describe
// their fields; scalar-only CHeader output remains independent of that header.
type Facade struct {
	CHeader, CSource []byte
	Functions        []string
}

// Generate renders an explicit facade without reading files or running tools.
// The compiler rejects non-unions, incompatible member types and native function
// prototypes. Header is a quoted-include path with forward slashes. The result
// supports at most 256 exports, 64 arguments per export and 1 MiB of source.
// Native exceptions cannot cross the facade; this is a C17 interface.
func Generate(header string, functions []Function) (*Facade, error) {
	if header == "" || len(header) > 4096 || strings.ContainsAny(header, "\"\\\r\n\x00") {
		return nil, fmt.Errorf("cunion: expected a quoted-include path using forward slashes")
	}
	if len(functions) == 0 || len(functions) > 256 {
		return nil, fmt.Errorf("cunion: select 1 to 256 functions")
	}
	seen, native := map[string]bool{}, map[string]bool{}
	for _, f := range functions {
		if !identifier(f.Name) || !identifier(f.Symbol) || seen[f.Symbol] || len(f.Args) > 64 {
			return nil, fmt.Errorf("cunion: invalid/duplicate export or argument count for %q", f.Symbol)
		}
		seen[f.Symbol], native[f.Name] = true, true
	}
	var declarations, definitions bytes.Buffer
	needsHeader := false
	result := &Facade{}
	for _, f := range functions {
		if native[f.Symbol] {
			return nil, fmt.Errorf("cunion: export %q conflicts with a selected native function", f.Symbol)
		}
		digest := sha256.Sum256([]byte(header + "\x00" + f.Name + "\x00" + f.Symbol))
		prefix := fmt.Sprintf("dylib_go_cunion_%x_", digest[:8])
		r, err := transport(f.Result, true)
		if err != nil {
			return nil, fmt.Errorf("cunion: %s result: %w", f.Symbol, err)
		}
		if f.Result.Record != "" {
			needsHeader = true
		}
		nativeResult := r
		if f.Result.Union != "" {
			nativeResult = f.Result.Union
		}
		var params, types, args []string
		var checks, body bytes.Buffer
		for i, v := range f.Args {
			kind, err := transport(v, false)
			if err != nil {
				return nil, fmt.Errorf("cunion: %s argument %d: %w", f.Symbol, i+1, err)
			}
			if v.Record != "" {
				needsHeader = true
				writeRecordCheck(&checks, v.Record)
			}
			params = append(params, fmt.Sprintf("%s %sa%d", kind, prefix, i))
			if v.Union == "" {
				types = append(types, kind)
				args = append(args, fmt.Sprintf("%sa%d", prefix, i))
				continue
			}
			types = append(types, v.Union)
			args = append(args, fmt.Sprintf("%sv%d", prefix, i))
			writeChecks(&checks, v, kind)
			fmt.Fprintf(&body, "%s %sv%d = {0};\n%sv%d.%s = %sa%d;\n", v.Union, prefix, i, prefix, i, v.Member, prefix, i)
		}
		if f.Result.Record != "" {
			writeRecordCheck(&checks, f.Result.Record)
		}
		if f.Result.Union != "" {
			writeChecks(&checks, f.Result, r)
		}
		if len(params) == 0 {
			params = []string{"void"}
		}
		if len(types) == 0 {
			types = []string{"void"}
		}
		fmt.Fprintf(&declarations, "DYLIB_GO_CUNION_API %s %s(%s);\n", r, f.Symbol, strings.Join(params, ", "))
		fmt.Fprint(&definitions, checks.String())
		fmt.Fprintf(&definitions, "typedef %s (*%stype)(%s);\n_Static_assert(_Generic(&%s, %stype: 1, default: 0), \"cunion: incompatible native function prototype\");\nstatic %stype const %starget = &%s;\n", nativeResult, prefix, strings.Join(types, ", "), f.Name, prefix, prefix, prefix, f.Name)
		fmt.Fprintf(&definitions, "DYLIB_GO_CUNION_API %s %s(%s) {\n%s", r, f.Symbol, strings.Join(params, ", "), body.String())
		call := prefix + "target(" + strings.Join(args, ", ") + ")"
		if f.Result.Union != "" {
			fmt.Fprintf(&definitions, "%s %svalue = %s;\nreturn %svalue.%s;\n", nativeResult, prefix, call, prefix, f.Result.Member)
		} else if f.Result.Type == abi.Void {
			fmt.Fprintf(&definitions, "%s;\n", call)
		} else {
			fmt.Fprintf(&definitions, "return %s;\n", call)
		}
		fmt.Fprintln(&definitions, "}")
		result.Functions = append(result.Functions, f.Symbol)
		if declarations.Len()+definitions.Len() > 1<<20 {
			return nil, fmt.Errorf("cunion: generated source exceeds 1 MiB")
		}
	}
	digest := sha256.Sum256(declarations.Bytes())
	guard := fmt.Sprintf("DYLIB_GO_CUNION_%X", digest[:8])
	api := "#if defined(_WIN32)\n#define DYLIB_GO_CUNION_API __declspec(dllexport) __cdecl\n#else\n#define DYLIB_GO_CUNION_API __attribute__((visibility(\"default\")))\n#endif\n"
	headerInclude, sourceInclude := "", fmt.Sprintf("#include \"%s\"\n", header)
	if needsHeader {
		headerInclude, sourceInclude = sourceInclude, ""
	}
	result.CHeader = []byte(fmt.Sprintf("#ifndef %s\n#define %s\n#include <stdint.h>\n#include <stdbool.h>\n%s%s#ifdef __cplusplus\nextern \"C\" {\n#endif\n%s#ifdef __cplusplus\n}\n#endif\n#endif\n", guard, guard, headerInclude, api, declarations.String()))
	result.CSource = []byte(fmt.Sprintf("%s%s\n%s", sourceInclude, result.CHeader, definitions.String()))
	if len(result.CSource) > 1<<20 {
		return nil, fmt.Errorf("cunion: generated source exceeds 1 MiB")
	}
	return result, nil
}

func writeChecks(out *bytes.Buffer, v Value, kind string) {
	fmt.Fprintf(out, "_Static_assert(__builtin_classify_type(*(%s *)0) == 13, \"cunion: selected type must be a union\");\n_Static_assert(_Generic(((%s *)0)->%s, %s: 1, default: 0), \"cunion: incompatible union member transport\");\n_Static_assert(sizeof(((%s *)0)->%s) == sizeof(%s), \"cunion: member requires full member storage\");\n", v.Union, v.Union, v.Member, kind, v.Union, v.Member, kind)
}

func writeRecordCheck(out *bytes.Buffer, name string) {
	fmt.Fprintf(out, "_Static_assert(__builtin_classify_type(*(%s *)0) == 12, \"cunion: record transport must name a struct\");\n", name)
}

func transport(v Value, result bool) (string, error) {
	if v.Type == abi.Struct {
		if !recordName(v.Record) {
			return "", fmt.Errorf("struct transport requires a C record name")
		}
	} else if v.Record != "" {
		return "", fmt.Errorf("record name requires a struct transport")
	}
	if v.Union == "" && v.Member != "" || v.Union != "" && (!unionName(v.Union) || !identifier(v.Member) || v.Type == abi.Void) {
		return "", fmt.Errorf("invalid union/member selection")
	}
	switch v.Type {
	case abi.Struct:
		return v.Record, nil
	case abi.Void:
		if result {
			return "void", nil
		}
	case abi.Bool:
		return "bool", nil
	case abi.I8:
		return "int8_t", nil
	case abi.U8:
		return "uint8_t", nil
	case abi.I16:
		return "int16_t", nil
	case abi.U16:
		return "uint16_t", nil
	case abi.I32:
		return "int32_t", nil
	case abi.U32:
		return "uint32_t", nil
	case abi.I64:
		return "int64_t", nil
	case abi.U64:
		return "uint64_t", nil
	case abi.F32:
		return "float", nil
	case abi.F64:
		return "double", nil
	case abi.Pointer:
		return "void *", nil
	}
	return "", fmt.Errorf("expected a scalar/native-address or named struct transport")
}

func recordName(s string) bool { return identifier(strings.TrimPrefix(s, "struct ")) }
func unionName(s string) bool  { return identifier(strings.TrimPrefix(s, "union ")) }
func identifier(s string) bool {
	if s == "" || len(s) > 256 {
		return false
	}
	for i, c := range s {
		if c != '_' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return !strings.Contains(" auto break case char const continue default do double else enum extern float for goto if inline int long register restrict return short signed sizeof static struct switch typedef union unsigned void volatile while _Alignas _Alignof _Atomic _Bool _Complex _Generic _Imaginary _Noreturn _Static_assert _Thread_local ", " "+s+" ")
}
