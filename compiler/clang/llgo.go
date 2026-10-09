package clang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// LLGoSource emits typed, fixed C cdecl bindings for qualified 64-bit llgo hosts.
// Scalar and opaque pointer calls use llgo's public C function-pointer directive
// without libffi. The caller owns the Session; each method retains its symbol for
// the native call. Ordinary records and typed native pointers use compiler-checked
// layouts. Callbacks, variadic signatures and other conventions are rejected.
func (h *Header) LLGoSource(packageName, bindingType string) ([]byte, error) {
	if h == nil || !validGoName(bindingType) {
		return nil, fmt.Errorf("clang: expected a header and valid binding type name")
	}
	if types.Universe.Lookup(bindingType) != nil {
		return nil, fmt.Errorf("clang: binding type conflicts with a predeclared Go name")
	}
	switch bindingType {
	case "abi", "clang", "dylib", "unsafe", "fmt":
		return nil, fmt.Errorf("clang: binding type conflicts with generated imports")
	}
	if h.Target.Arch != "amd64" && h.Target.Arch != "arm64" {
		return nil, fmt.Errorf("clang: direct llgo adapters require a qualified amd64/arm64 target")
	}
	metadata := "_" + bindingType + "Declarations"
	source, err := h.GoSource(packageName, metadata)
	if err != nil {
		return nil, err
	}
	mapper, err := newDirectTypes(h.Records, bindingType)
	if err != nil {
		return nil, err
	}
	methodNames := make(map[string]bool)
	var methods []string
	for _, function := range h.Functions {
		s := function.Signature
		if s.Variadic || (s.Convention != abi.Default && s.Convention != abi.CDecl) || len(function.FunctionPointers) != 0 {
			return nil, fmt.Errorf("clang: %s requires a fixed cdecl adapter", function.Name)
		}
		if _, err := mapper.goType(s.ReturnType()); err != nil {
			return nil, fmt.Errorf("clang: %s result: %w", function.Name, err)
		}
		for i := range s.Args {
			if _, err := mapper.goType(s.ArgumentType(i)); err != nil {
				return nil, fmt.Errorf("clang: %s argument %d: %w", function.Name, i, err)
			}
		}
		name := directName(function.Name)
		if methodNames[name] {
			return nil, fmt.Errorf("clang: generated method name %s is ambiguous", name)
		}
		methodNames[name] = true
		methods = append(methods, name)
	}
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "generated.go", source, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	for _, declaration := range file.Decls {
		if imports, ok := declaration.(*ast.GenDecl); ok && imports.Tok == token.IMPORT {
			for _, path := range []string{"unsafe", "github.com/cpunion/dylib-go"} {
				spec := &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(path)}}
				if path != "unsafe" {
					spec.Name = ast.NewIdent("dylib")
				}
				imports.Specs = append(imports.Specs, spec)
			}
			if len(h.Records) != 0 {
				imports.Specs = append(imports.Specs, &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("fmt")}})
			}
		}
	}
	var output bytes.Buffer
	if err := format.Node(&output, set, file); err != nil {
		return nil, err
	}
	mapper.writeRecords(&output)
	fmt.Fprintf(&output, "\n// %s retains symbol handles; its caller owns the Session.\ntype %s struct {\n", bindingType, bindingType)
	for i := range h.Functions {
		fmt.Fprintf(&output, "symbol%d *dylib.Symbol\n", i)
	}
	fmt.Fprintf(&output, "}\n\n// New%s validates the host and resolves each requested export.\nfunc New%s(session *dylib.Session) (*%s,error) {\nif session == nil { return nil,dylib.ErrClosed }\nif err := %s.Target.CheckHost(); err != nil { return nil,err }\nb := &%s{}\n", bindingType, bindingType, bindingType, metadata, bindingType)
	mapper.writeLayoutChecks(&output)
	for i, function := range h.Functions {
		fmt.Fprintf(&output, "symbol%d,err := session.Resolve(%q)\nif err != nil { return nil,err }\nb.symbol%d = symbol%d\n", i, function.Symbol, i, i)
	}
	fmt.Fprint(&output, "return b,nil\n}\n")
	for i, function := range h.Functions {
		result, _ := mapper.goType(function.Signature.ReturnType())
		var parameters, arguments []string
		for j := range function.Signature.Args {
			kind, _ := mapper.goType(function.Signature.ArgumentType(j))
			parameters = append(parameters, fmt.Sprintf("p%d %s", j, kind))
			arguments = append(arguments, fmt.Sprintf("p%d", j))
		}
		params, args := strings.Join(parameters, ","), strings.Join(arguments, ",")
		fmt.Fprintf(&output, "\n//llgo:type C\ntype _%sFunction%d func(%s) %s\n", bindingType, i, params, result)
		if result == "" {
			fmt.Fprintf(&output, "\nfunc (b *%s) %s(%s) error {\nif b == nil { return dylib.ErrClosed }\nreturn b.symbol%d.WithAddress(func(address uintptr) error {\nfunction := *(*_%sFunction%d)(unsafe.Pointer(&address))\nfunction(%s)\nreturn nil\n})\n}\n", bindingType, methods[i], params, i, bindingType, i, args)
		} else {
			fmt.Fprintf(&output, "\nfunc (b *%s) %s(%s) (result %s,err error) {\nif b == nil { return result,dylib.ErrClosed }\nerr = b.symbol%d.WithAddress(func(address uintptr) error {\nfunction := *(*_%sFunction%d)(unsafe.Pointer(&address))\nresult = function(%s)\nreturn nil\n})\nreturn result,err\n}\n", bindingType, methods[i], params, result, i, bindingType, i, args)
		}
	}
	source = bytes.Replace(output.Bytes(), []byte("\npackage "), []byte("\n//go:build llgo && cgo\n\npackage "), 1)
	return format.Source(source)
}

func directGoType(description abi.TypeDesc) (string, error) {
	if description.Elem != nil || description.Type == abi.Struct || description.Type == abi.Array {
		return "", fmt.Errorf("direct llgo generation supports scalars and opaque native pointers")
	}
	return map[abi.Type]string{abi.Void: "", abi.I8: "int8", abi.U8: "uint8", abi.I16: "int16", abi.U16: "uint16", abi.I32: "int32", abi.U32: "uint32", abi.I64: "int64", abi.U64: "uint64", abi.F32: "float32", abi.F64: "float64", abi.Bool: "bool", abi.Pointer: "unsafe.Pointer"}[description.Type], nil
}
