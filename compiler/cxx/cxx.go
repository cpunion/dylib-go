// Package cxx generates explicit C facades for selected C++ classes. A matching
// C++ compiler supplies class layout, member calls, virtual dispatch and runtime
// support. The generated C declarations can use compiler/clang's Go/llgo bindings.
package cxx

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// Status values returned by every generated C entry.
const (
	Success         int32 = 0
	InvalidArgument int32 = 1
	Exception       int32 = 2
)

// Constructor selects a C export and the argument types used for C++ overload
// resolution. The export appends a void** output slot, which must initially be null.
type Constructor struct {
	Symbol string
	Args   []abi.Type
}

// Method selects an exact scalar/opaque-pointer C++ member signature. Const
// selects a const-qualified member; Static omits the receiver. A non-void result
// adds an output pointer after the arguments. Variadics and aggregate types are
// unsupported. Returned native pointers are borrowed unless the native API says
// otherwise; generation does not infer ownership.
type Method struct {
	Name, Symbol  string
	Signature     abi.Signature
	Const, Static bool
}

// Class names an accessible C++ type, including namespace-qualified aliases.
// Delete names the C export that consumes and clears a void** handle slot before
// invoking delete. Borrowed base views must not be deleted unless the native
// contract permits it (including a virtual destructor when required).
type Class struct {
	Name         string
	Delete       string
	Constructors []Constructor
	Methods      []Method
}

// Facade contains independent source buffers and the ordered C export names.
// Write CHeader for compiler/clang.Parse with Functions, and compile CXXSource
// against the original header with a matching C++17 compiler/runtime. The source
// embeds its C declarations, so its output filename is independent of CHeader's.
type Facade struct {
	CHeader, CXXSource []byte
	Functions          []string
}

const apiMacro = `#if defined(_WIN32)
#define DYLIB_GO_CXX_API __declspec(dllexport) int32_t __cdecl
#else
#define DYLIB_GO_CXX_API __attribute__((visibility("default"))) int32_t
#endif
#ifdef __cplusplus
#define DYLIB_GO_CXX_NOEXCEPT noexcept
#else
#define DYLIB_GO_CXX_NOEXCEPT
#endif
`

// Generate renders a C facade without invoking a compiler or reading files.
// Header is a quoted-include path using forward slashes; include directories
// belong to the producer compiler. Compile errors reject missing/private members,
// signature mismatches and incompatible class definitions. This is an explicit
// manifest, not a C++ header parser. Constructors use C++ overload resolution;
// methods use an exact typed member/function pointer.
//
// Exported functions catch C++ exceptions and return Exception. This cannot
// recover from noexcept violations, termination, signals or invalid addresses.
// Objects, output storage, provider images and asynchronous native activity stay
// caller-owned. Use complete OS libraries for C++ runtime/exception registration.
func Generate(header string, classes []Class) (*Facade, error) {
	if header == "" || strings.ContainsAny(header, "\"\\\r\n\x00") {
		return nil, fmt.Errorf("cxx: expected a quoted-include path using forward slashes")
	}
	if len(classes) == 0 {
		return nil, fmt.Errorf("cxx: select at least one class")
	}
	var declarations, definitions bytes.Buffer
	result := &Facade{}
	seen := make(map[string]bool)
	emit := func(symbol string, parameters []string, body string) error {
		if !identifier(symbol) || seen[symbol] {
			return fmt.Errorf("cxx: invalid or duplicate C export %q", symbol)
		}
		if len(result.Functions) == 256 {
			return fmt.Errorf("cxx: at most 256 C exports are supported")
		}
		seen[symbol] = true
		result.Functions = append(result.Functions, symbol)
		params := strings.Join(parameters, ", ")
		if params == "" {
			params = "void"
		}
		fmt.Fprintf(&declarations, "DYLIB_GO_CXX_API %s(%s) DYLIB_GO_CXX_NOEXCEPT;\n", symbol, params)
		fmt.Fprintf(&definitions, "extern \"C\" DYLIB_GO_CXX_API %s(%s) noexcept {\n%s}\n", symbol, params, body)
		return nil
	}
	for _, class := range classes {
		if !qualifiedName(class.Name) {
			return nil, fmt.Errorf("cxx: invalid C++ type name %q", class.Name)
		}
		for _, constructor := range class.Constructors {
			parameters, _, args, err := arguments(constructor.Args)
			if err != nil {
				return nil, fmt.Errorf("cxx: constructor %s: %w", constructor.Symbol, err)
			}
			parameters = append(parameters, "void **output")
			body := fmt.Sprintf(`if (!output || *output) return DYLIB_CXX_INVALID_ARGUMENT;
try {
auto object = new %s(%s);
if (!object) return DYLIB_CXX_EXCEPTION;
*output = object;
return DYLIB_CXX_SUCCESS;
} catch (...) { return DYLIB_CXX_EXCEPTION; }
`, class.Name, args)
			if err := emit(constructor.Symbol, parameters, body); err != nil {
				return nil, err
			}
		}
		for _, method := range class.Methods {
			if !identifier(method.Name) || method.Const && method.Static {
				return nil, fmt.Errorf("cxx: invalid method %q or static/const combination", method.Name)
			}
			if err := methodSignature(method.Signature); err != nil {
				return nil, fmt.Errorf("cxx: method %s: %w", method.Name, err)
			}
			parameters, types, args, _ := arguments(method.Signature.Args)
			var body bytes.Buffer
			if !method.Static {
				parameters = append([]string{"void *handle"}, parameters...)
			}
			returns := method.Signature.Result != abi.Void
			kind, _ := cType(method.Signature.Result)
			if returns {
				parameters = append(parameters, kind+" *output")
				fmt.Fprint(&body, "if (!output) return DYLIB_CXX_INVALID_ARGUMENT;\n*output = {};\n")
			}
			if !method.Static {
				fmt.Fprint(&body, "if (!handle) return DYLIB_CXX_INVALID_ARGUMENT;\n")
			}
			fmt.Fprint(&body, "try {\n")
			call := "entry(" + args + ")"
			if method.Static {
				fmt.Fprintf(&body, "using entry_type = %s (*)(%s);\n", kind, types)
			} else {
				qualifier := ""
				if method.Const {
					qualifier = " const"
				}
				fmt.Fprintf(&body, "using entry_type = %s (%s::*)(%s)%s;\n", kind, class.Name, types, qualifier)
				receiver := class.Name
				if method.Const {
					receiver = "const " + receiver
				}
				call = "(static_cast<" + receiver + " *>(handle)->*entry)(" + args + ")"
			}
			fmt.Fprintf(&body, "auto entry = static_cast<entry_type>(&%s::%s);\n", class.Name, method.Name)
			if returns {
				fmt.Fprint(&body, "*output = ")
			}
			fmt.Fprintf(&body, "%s;\nreturn DYLIB_CXX_SUCCESS;\n} catch (...) { return DYLIB_CXX_EXCEPTION; }\n", call)
			if err := emit(method.Symbol, parameters, body.String()); err != nil {
				return nil, err
			}
		}
		body := fmt.Sprintf(`if (!handle) return DYLIB_CXX_INVALID_ARGUMENT;
auto object = static_cast<%s *>(*handle);
*handle = nullptr;
if (!object) return DYLIB_CXX_SUCCESS;
try {
delete object;
return DYLIB_CXX_SUCCESS;
} catch (...) { return DYLIB_CXX_EXCEPTION; }
`, class.Name)
		if err := emit(class.Delete, []string{"void **handle"}, body); err != nil {
			return nil, err
		}
	}
	guard := fmt.Sprintf("DYLIB_GO_CXX_FACADE_%X", sha256.Sum256(declarations.Bytes()))
	var c bytes.Buffer
	fmt.Fprintf(&c, "// Code generated by compiler/cxx. DO NOT EDIT.\n#ifndef %s\n#define %s\n#include <stdint.h>\n#include <stdbool.h>\n", guard, guard)
	fmt.Fprintf(&c, "#ifndef DYLIB_GO_CXX_STATUS_DEFINED\n#define DYLIB_GO_CXX_STATUS_DEFINED\nenum { DYLIB_CXX_SUCCESS = %d, DYLIB_CXX_INVALID_ARGUMENT = %d, DYLIB_CXX_EXCEPTION = %d };\n#endif\n", Success, InvalidArgument, Exception)
	fmt.Fprint(&c, apiMacro, "#ifdef __cplusplus\nextern \"C\" {\n#endif\n", declarations.String(), "#ifdef __cplusplus\n}\n#endif\n#undef DYLIB_GO_CXX_API\n#undef DYLIB_GO_CXX_NOEXCEPT\n#endif\n")
	result.CHeader = c.Bytes()
	var cpp bytes.Buffer
	fmt.Fprintf(&cpp, "%s\n#include \"%s\"\n%s%s#undef DYLIB_GO_CXX_API\n#undef DYLIB_GO_CXX_NOEXCEPT\n", result.CHeader, header, apiMacro, definitions.String())
	result.CXXSource = cpp.Bytes()
	return result, nil
}

func methodSignature(s abi.Signature) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.Variadic || s.Convention != abi.Default && s.Convention != abi.CDecl {
		return fmt.Errorf("expected a fixed default/cdecl signature")
	}
	for _, d := range append(append([]abi.TypeDesc(nil), s.ArgTypes...), s.ReturnType()) {
		if d.Elem != nil {
			return fmt.Errorf("pointers must be opaque native addresses")
		}
	}
	if _, _, _, err := arguments(s.Args); err != nil {
		return err
	}
	_, err := cType(s.Result)
	return err
}

func arguments(args []abi.Type) (parameters []string, types, values string, err error) {
	var names, kinds []string
	for i, arg := range args {
		kind, e := cType(arg)
		if e != nil || arg == abi.Void {
			return nil, "", "", fmt.Errorf("unsupported argument %d type %d", i, arg)
		}
		name := fmt.Sprintf("p%d", i)
		parameters, names, kinds = append(parameters, kind+" "+name), append(names, name), append(kinds, kind)
	}
	return parameters, strings.Join(kinds, ", "), strings.Join(names, ", "), nil
}

var cTypes = map[abi.Type]string{
	abi.Void: "void", abi.Bool: "bool", abi.Pointer: "void *",
	abi.I8: "int8_t", abi.U8: "uint8_t", abi.I16: "int16_t", abi.U16: "uint16_t",
	abi.I32: "int32_t", abi.U32: "uint32_t", abi.I64: "int64_t", abi.U64: "uint64_t",
	abi.F32: "float", abi.F64: "double",
}

func cType(t abi.Type) (string, error) {
	if kind := cTypes[t]; kind != "" {
		return kind, nil
	}
	return "", fmt.Errorf("unsupported scalar/opaque-pointer type %d", t)
}

func qualifiedName(s string) bool {
	for _, part := range strings.Split(s, "::") {
		if !identifier(part) {
			return false
		}
	}
	return true
}

func identifier(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if c != '_' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(i != 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return !keywords[s]
}

var keywords = func() map[string]bool {
	words := `alignas alignof and and_eq asm auto bitand bitor bool break case catch
char char8_t char16_t char32_t class compl co_await co_return co_yield concept
const consteval constexpr constinit const_cast continue decltype default delete
do double dynamic_cast else enum explicit export extern false float for friend
goto if inline int long mutable namespace new noexcept not not_eq nullptr
operator or or_eq private protected public register reinterpret_cast requires
return short signed sizeof static static_assert static_cast struct switch
template this thread_local throw true try typedef typeid typename union unsigned
using virtual void volatile wchar_t while xor xor_eq _Alignas _Alignof _Atomic
_Bool _Complex _Generic _Imaginary _Noreturn _Static_assert _Thread_local restrict`
	result := make(map[string]bool)
	for _, word := range strings.Fields(words) {
		result[word] = true
	}
	return result
}()
