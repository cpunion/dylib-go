package clang

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ramblelabs/dylib-go/abi"
)

var errEnumTypes = errors.New("clang: enums require compiler integer probes")

type enumType struct {
	spelling string
	complete bool
	probe    int
}

func enumReference(node astNode) string {
	if node.Kind == "EnumType" && node.Decl != nil {
		return node.Decl.ID
	}
	for _, child := range node.Inner {
		if id := enumReference(child); id != "" {
			return id
		}
	}
	return ""
}

func indexEnums(root astNode) map[string]*enumType {
	byID, names := make(map[string]*enumType), make(map[string]*enumType)
	var index func(astNode)
	index = func(node astNode) {
		if node.Kind == "EnumDecl" {
			e := &enumType{complete: node.FixedUnderlyingType.QualType != ""}
			for _, child := range node.Inner {
				e.complete = e.complete || child.Kind == "EnumConstantDecl"
			}
			byID[node.ID] = e
			if cIdentifier(node.Name) {
				e.spelling = "enum " + node.Name
				if previous := names[e.spelling]; previous == nil || e.complete {
					names[e.spelling] = e
				}
			}
		} else if node.Kind == "TranslationUnitDecl" || node.Kind == "RecordDecl" {
			for _, child := range node.Inner {
				index(child)
			}
		}
	}
	index(root)
	for _, node := range root.Inner {
		if node.Kind != "TypedefDecl" || !cIdentifier(node.Name) || strings.ContainsAny(node.Type.spelling(), "*[]()") {
			continue
		}
		e := byID[enumReference(node)]
		if e == nil {
			continue
		}
		if previous := names[e.spelling]; previous != nil && previous.complete {
			e = previous
		}
		if e.spelling == "" {
			e.spelling = node.Name
		}
		names[node.Name] = e
		spelling := unqualified(node.Type.spelling())
		// An anonymous typedef enum can have a synthetic spelling identical
		// to an unrelated real tag. Its typedef name remains unambiguous.
		if names[spelling] == nil || names[spelling] == e {
			names[spelling] = e
		}
	}
	// Clang node IDs change on each invocation. Canonical C spellings keep
	// probe numbers stable after adding the anonymous probe enumeration.
	var spellings []string
	seen := make(map[*enumType]bool)
	for _, e := range names {
		if !seen[e] {
			seen[e] = true
			spellings = append(spellings, e.spelling)
		}
	}
	sort.Strings(spellings)
	for i, spelling := range spellings {
		names[spelling].probe = i
	}
	return names
}

// Observe the enum's actual compatible C integer type, rather than the types
// of its enumerator constants (which lose packed and short-enum information).
func enumProbes(data []byte) (string, error) {
	var root astNode
	if err := json.Unmarshal(data, &root); err != nil {
		return "", err
	}
	enums := indexEnums(root)
	var spellings []string
	for name, e := range enums {
		if name == e.spelling && e.complete {
			spellings = append(spellings, name)
		}
	}
	sort.Strings(spellings)
	var source strings.Builder
	for _, name := range spellings {
		e := enums[name]
		alignment := "0"
		for _, integer := range []string{"char", "signed char", "unsigned char", "short", "unsigned short", "int", "unsigned int", "long", "unsigned long", "long long", "unsigned long long", "_Bool"} {
			alignment = fmt.Sprintf("__builtin_types_compatible_p(%s,%s)?_Alignof(%s):(%s)", name, integer, integer, alignment)
		}
		fmt.Fprintf(&source, "enum { dylib_probe_enum_%d_size=sizeof(%s), dylib_probe_enum_%d_align=_Alignof(%s), dylib_probe_enum_%d_unsigned=((%s)-1>0), dylib_probe_enum_%d_bool=__builtin_types_compatible_p(%s,_Bool), dylib_probe_enum_%d_native_align=%s };\n", e.probe, name, e.probe, name, e.probe, name, e.probe, name, e.probe, alignment)
		if source.Len() > maxHeaderSize {
			return "", fmt.Errorf("clang: enum probes exceed source limit")
		}
	}
	return source.String(), nil
}

func (r *typeResolver) describeEnum(e *enumType) (abi.TypeDesc, error) {
	if !e.complete {
		return abi.TypeDesc{}, fmt.Errorf("unsupported incomplete C enum %s", e.spelling)
	}
	prefix := fmt.Sprintf("enum_%d_", e.probe)
	get := func(key string) (int, error) {
		value, ok := r.sizes[prefix+key]
		if !ok {
			return 0, errEnumTypes
		}
		return value, nil
	}
	values := make(map[string]int)
	for _, key := range []string{"size", "align", "unsigned", "bool", "native_align"} {
		value, err := get(key)
		if err != nil {
			return abi.TypeDesc{}, err
		}
		values[key] = value
	}
	if values["native_align"] == 0 || values["align"] != values["native_align"] {
		return abi.TypeDesc{}, fmt.Errorf("unsupported C enum alignment or integer type for %s", e.spelling)
	}
	if values["bool"] == 1 && values["size"] == 1 {
		return abi.TypeDesc{Type: abi.Bool}, nil
	}
	pair, ok := map[int][2]abi.Type{1: {abi.I8, abi.U8}, 2: {abi.I16, abi.U16}, 4: {abi.I32, abi.U32}, 8: {abi.I64, abi.U64}}[values["size"]]
	if !ok || values["bool"] != 0 || values["unsigned"] < 0 || values["unsigned"] > 1 {
		return abi.TypeDesc{}, fmt.Errorf("unsupported C enum integer for %s", e.spelling)
	}
	return abi.TypeDesc{Type: pair[values["unsigned"]]}, nil
}
