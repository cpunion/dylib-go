package clang

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

const sizeProbe = `enum {
 dylib_probe_char = sizeof(char),
 dylib_probe_short = sizeof(short),
 dylib_probe_int = sizeof(int),
 dylib_probe_long = sizeof(long),
 dylib_probe_longlong = sizeof(long long),
 dylib_probe_float = sizeof(float),
 dylib_probe_double = sizeof(double),
 dylib_probe_bool = sizeof(_Bool),
 dylib_probe_pointer = sizeof(void *),
 dylib_probe_char_unsigned = ((char)-1 > 0)
};
`

type nodeType struct {
	QualType          string `json:"qualType"`
	DesugaredQualType string `json:"desugaredQualType"`
}

func (t nodeType) spelling() string {
	if t.DesugaredQualType != "" {
		return t.DesugaredQualType
	}
	return t.QualType
}

type astNode struct {
	ID, Kind, Name, MangledName, StorageClass, Value, TagUsed, CC string
	Type                                                          nodeType
	Inline, Variadic, CompleteDefinition, IsBitfield              bool
	Decl                                                          *astNode
	Inner                                                         []astNode
}

func decodeDeclarations(data []byte, triple string, names []string) (*Header, error) {
	var root astNode
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("clang: invalid AST JSON: %w", err)
	}
	if root.Kind != "TranslationUnitDecl" {
		return nil, fmt.Errorf("clang: expected a translation unit AST")
	}
	aliases, sizes := make(map[string]string), make(map[string]int)
	for _, node := range root.Inner {
		if node.Kind == "TypedefDecl" {
			aliases[node.Name] = node.Type.spelling()
		}
		if node.Kind == "EnumDecl" {
			for _, field := range node.Inner {
				if strings.HasPrefix(field.Name, "dylib_probe_") {
					value, ok := constantValue(field)
					if !ok {
						return nil, fmt.Errorf("clang: missing evaluated size probe %s", field.Name)
					}
					sizes[strings.TrimPrefix(field.Name, "dylib_probe_")] = value
				}
			}
		}
	}
	for _, name := range []string{"char", "short", "int", "long", "longlong", "float", "double", "bool", "pointer", "char_unsigned"} {
		if _, ok := sizes[name]; !ok {
			return nil, fmt.Errorf("clang: missing size probe %s", name)
		}
	}
	if sizes["char"] != 1 || sizes["bool"] != 1 || sizes["float"] != 4 || sizes["double"] != 8 || sizes["char_unsigned"] < 0 || sizes["char_unsigned"] > 1 {
		return nil, fmt.Errorf("clang: unsupported primitive ABI sizes")
	}
	target, err := parseTarget(strings.TrimSpace(triple), sizes["pointer"])
	if err != nil {
		return nil, err
	}
	requested := make(map[string]bool)
	for _, name := range names {
		requested[name] = true
	}
	found := make(map[string]Declaration)
	resolver := newTypeResolver(root, aliases, sizes)
	prototypes := make(map[string]astNode)
	for _, node := range root.Inner {
		if node.Kind == "TypedefDecl" && strings.HasPrefix(node.Name, "dylib_function_probe_") {
			prototypes[node.Name] = node
		}
	}
	indices := make(map[string]int)
	for i, name := range names {
		indices[name] = i
	}
	for _, node := range root.Inner {
		if node.Kind != "FunctionDecl" || !requested[node.Name] {
			continue
		}
		declaration, err := decodeFunction(node, target, resolver, prototypes[fmt.Sprintf("dylib_function_probe_%d", indices[node.Name])])
		if err != nil {
			return nil, fmt.Errorf("clang: %s: %w", node.Name, err)
		}
		if previous, ok := found[node.Name]; ok && !reflect.DeepEqual(previous, declaration) {
			return nil, fmt.Errorf("clang: incompatible declarations for %s", node.Name)
		}
		found[node.Name] = declaration
	}
	header := &Header{Target: target, Records: resolver.records}
	for _, name := range names {
		declaration, ok := found[name]
		if !ok {
			return nil, fmt.Errorf("clang: no external function declaration for %q", name)
		}
		header.Functions = append(header.Functions, declaration)
	}
	if err := header.validateDescriptions(); err != nil {
		return nil, err
	}
	return header, nil
}

func constantValue(node astNode) (int, bool) {
	if node.Kind == "ConstantExpr" {
		value, err := strconv.Atoi(node.Value)
		return value, err == nil
	}
	for _, child := range node.Inner {
		if value, ok := constantValue(child); ok {
			return value, true
		}
	}
	return 0, false
}

func parseTarget(triple string, pointerSize int) (Target, error) {
	target := Target{Triple: triple, PointerSize: pointerSize}
	arch, _, _ := strings.Cut(triple, "-")
	switch arch {
	case "x86_64", "amd64":
		target.Arch = "amd64"
	case "aarch64", "arm64":
		target.Arch = "arm64"
	case "i386", "i486", "i586", "i686":
		target.Arch = "386"
	}
	switch {
	case strings.Contains(triple, "-linux-") && !strings.Contains(triple, "android"):
		target.OS = "linux"
	case strings.Contains(triple, "-windows-"):
		target.OS = "windows"
	case strings.Contains(triple, "-apple-macos") || strings.Contains(triple, "-apple-darwin"):
		target.OS = "darwin"
	}
	if target.Arch == "" || target.OS == "" || target.OS == "darwin" && target.Arch == "386" || pointerSize != 4 && pointerSize != 8 || (target.Arch == "386") != (pointerSize == 4) {
		return Target{}, fmt.Errorf("clang: unsupported target/ABI %q (%d-byte pointers)", triple, pointerSize)
	}
	return target, nil
}

func decodeFunction(node astNode, target Target, resolver *typeResolver, probe astNode) (Declaration, error) {
	if node.StorageClass == "static" || node.Inline {
		return Declaration{}, fmt.Errorf("static/inline functions require an exported facade")
	}
	prototype, err := resolver.effectiveType(probe)
	if err != nil {
		return Declaration{}, err
	}
	signature, pointers, err := resolver.prototype(prototype, target, true)
	if err != nil {
		return Declaration{}, err
	}

	symbol := node.MangledName
	if symbol == "" {
		return Declaration{}, fmt.Errorf("missing compiler symbol spelling")
	}
	// Match the existing object parsers' C underscore normalization, retaining
	// Windows i386 stdcall suffixes and fastcall prefixes.
	if target.OS == "darwin" || target.OS == "windows" && target.Arch == "386" {
		symbol = strings.TrimPrefix(symbol, "_")
	}
	return Declaration{Name: node.Name, Symbol: symbol, Signature: signature, FunctionPointers: pointers}, nil
}

func scalarType(spelling string, aliases map[string]string, sizes map[string]int, depth int) (abi.Type, error) {
	if depth > 16 {
		return abi.Void, fmt.Errorf("typedef nesting exceeds 16 levels")
	}
	var words []string
	for _, word := range strings.Fields(strings.ReplaceAll(spelling, "*", " * ")) {
		switch word {
		case "const", "volatile", "restrict", "__restrict", "__restrict__":
		default:
			words = append(words, word)
		}
	}
	spelling = strings.Join(words, " ")
	if strings.ContainsAny(spelling, "([])") || strings.Contains(spelling, "_Atomic") {
		return abi.Void, fmt.Errorf("unsupported C type %q", spelling)
	}
	if strings.HasSuffix(spelling, " *") {
		base := strings.TrimSpace(strings.TrimRight(spelling, "* "))
		for i := 0; i < 16; i++ {
			alias, ok := aliases[base]
			if !ok {
				break
			}
			if strings.ContainsAny(alias, "([])") {
				return abi.Void, fmt.Errorf("unsupported pointer alias %q", base)
			}
			base = strings.TrimSpace(strings.TrimRight(alias, "* "))
		}
		return abi.Pointer, nil // Opaque native address, no inferred pointee layout.
	}
	if alias, ok := aliases[spelling]; ok {
		return scalarType(alias, aliases, sizes, depth+1)
	}
	unsigned, size := false, 0
	switch spelling {
	case "void":
		return abi.Void, nil
	case "_Bool":
		return abi.Bool, nil
	case "float":
		return abi.F32, nil
	case "double":
		return abi.F64, nil
	case "char":
		size, unsigned = sizes["char"], sizes["char_unsigned"] != 0
	case "signed char":
		size = sizes["char"]
	case "unsigned char":
		size, unsigned = sizes["char"], true
	case "short", "short int", "signed short", "signed short int":
		size = sizes["short"]
	case "unsigned short", "unsigned short int":
		size, unsigned = sizes["short"], true
	case "int", "signed", "signed int":
		size = sizes["int"]
	case "unsigned", "unsigned int":
		size, unsigned = sizes["int"], true
	case "long", "long int", "signed long", "signed long int":
		size = sizes["long"]
	case "unsigned long", "unsigned long int":
		size, unsigned = sizes["long"], true
	case "long long", "long long int", "signed long long", "signed long long int":
		size = sizes["longlong"]
	case "unsigned long long", "unsigned long long int":
		size, unsigned = sizes["longlong"], true
	default:
		return abi.Void, fmt.Errorf("unsupported C type %q", spelling)
	}
	ids := map[int][2]abi.Type{1: {abi.I8, abi.U8}, 2: {abi.I16, abi.U16}, 4: {abi.I32, abi.U32}, 8: {abi.I64, abi.U64}}
	pair, ok := ids[size]
	if !ok {
		return abi.Void, fmt.Errorf("unsupported %d-byte C integer", size)
	}
	if unsigned {
		return pair[1], nil
	}
	return pair[0], nil
}
