// Package signature parses Go-style C ABI declarations and typed values.
// It does not infer native prototypes or expose raw Go memory to native code.
package signature

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// Declaration is a symbol name paired with a caller-supplied C ABI signature.
type Declaration struct {
	Name      string
	Signature abi.Signature
}

// Parse accepts a single Go function declaration without a body. Supported
// types are fixed-width integers, bool, float32/64, uintptr, unsafe.Pointer,
// pointers, and inline ordinary C structs. An omitted result is void.
// uintptr follows the host word size; int and uint are deliberately rejected.
func Parse(text string) (Declaration, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "signature", "package signature\n"+text, parser.SkipObjectResolution)
	if err != nil {
		return Declaration{}, fmt.Errorf("invalid function signature: %w", err)
	}
	if len(f.Decls) != 1 {
		return Declaration{}, fmt.Errorf("expected one function declaration")
	}
	fn, ok := f.Decls[0].(*ast.FuncDecl)
	if !ok || fn.Recv != nil || fn.Body != nil || fn.Type.TypeParams != nil || fn.Name.Name == "_" {
		return Declaration{}, fmt.Errorf("expected a named function without receiver, body, or type parameters")
	}
	d := Declaration{Name: fn.Name.Name}
	var descriptions []abi.TypeDesc
	complex := false
	for _, field := range fn.Type.Params.List {
		t, err := describeType(field.Type)
		if err != nil {
			return Declaration{}, err
		}
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for i := 0; i < count; i++ {
			d.Signature.Args = append(d.Signature.Args, t.Type)
			descriptions = append(descriptions, t)
			complex = complex || t.Type == abi.Struct || t.Elem != nil
		}
	}
	if fn.Type.Results.NumFields() > 1 {
		return Declaration{}, fmt.Errorf("C ABI calls support at most one result")
	}
	if fn.Type.Results.NumFields() == 1 {
		var result abi.TypeDesc
		result, err = describeType(fn.Type.Results.List[0].Type)
		d.Signature.Result = result.Type
		if result.Type == abi.Struct || result.Elem != nil {
			d.Signature.ResultType = &result
		}
		if err != nil {
			return Declaration{}, err
		}
	}
	if complex {
		d.Signature.ArgTypes = descriptions
	}
	if err := d.Signature.Validate(); err != nil {
		return Declaration{}, err
	}
	return d, nil
}

func describeType(expr ast.Expr) (abi.TypeDesc, error) {
	switch t := expr.(type) {
	case *ast.Ident:
		types := map[string]abi.Type{"bool": abi.Bool, "int8": abi.I8, "uint8": abi.U8, "byte": abi.U8, "int16": abi.I16, "uint16": abi.U16, "int32": abi.I32, "rune": abi.I32, "uint32": abi.U32, "int64": abi.I64, "uint64": abi.U64, "float32": abi.F32, "float64": abi.F64}
		if kind, ok := types[t.Name]; ok {
			return abi.TypeDesc{Type: kind}, nil
		}
		if t.Name == "uintptr" {
			kind := abi.U64
			if strconv.IntSize == 32 {
				kind = abi.U32
			}
			return abi.TypeDesc{Type: kind}, nil
		}
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok && pkg.Name == "unsafe" && t.Sel.Name == "Pointer" {
			return abi.TypeDesc{Type: abi.Pointer}, nil
		}
	case *ast.StarExpr:
		d := abi.TypeDesc{Type: abi.Pointer}
		elem, err := describeType(t.X)
		if err != nil {
			if _, opaque := t.X.(*ast.Ident); opaque {
				return d, nil
			}
			return d, err
		}
		d.Elem = &elem
		return d, nil
	case *ast.ParenExpr:
		return describeType(t.X)
	case *ast.StructType:
		d := abi.TypeDesc{Type: abi.Struct}
		for _, field := range t.Fields.List {
			child, err := describeType(field.Type)
			if err != nil {
				return abi.TypeDesc{}, err
			}
			if len(field.Names) == 0 {
				return abi.TypeDesc{}, fmt.Errorf("C struct fields must be explicitly named")
			}
			for _, name := range field.Names {
				d.Fields = append(d.Fields, abi.Field{Name: name.Name, Type: child})
			}
		}
		if err := d.Validate(); err != nil {
			return abi.TypeDesc{}, err
		}
		return d, nil
	}
	return abi.TypeDesc{}, fmt.Errorf("unsupported native type; use fixed-width integers, bool, floats, pointers, or ordinary structs")
}

// Invocation includes literals in the form name(value:type, value:type)result.
type Invocation struct {
	Declaration
	Args []abi.Value
}

// ParseCall accepts whitespace or commas between typed arguments. Values are
// typed literals, not Go expressions. Types and the optional result are
// validated by Parse, so both input forms share the same ABI rules.
func ParseCall(text string) (Invocation, error) {
	name, types, values, result, err := splitInvocation(text)
	if err != nil {
		return Invocation{}, err
	}
	d, err := Parse("func " + name + "(" + strings.Join(types, ",") + ")" + result)
	if err != nil {
		return Invocation{}, err
	}
	if len(d.Signature.Args) != len(values) {
		return Invocation{}, fmt.Errorf("each argument needs one type")
	}
	call := Invocation{Declaration: d}
	for i, text := range values {
		v, err := ParseTypedValue(d.Signature.ArgumentType(i), text)
		if err != nil {
			return Invocation{}, fmt.Errorf("argument %d: %w", i+1, err)
		}
		call.Args = append(call.Args, v)
	}
	return call, nil
}
