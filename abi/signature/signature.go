// Package signature parses Go-style C ABI declarations and scalar values.
// It does not infer native prototypes or marshal Go-managed values.
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
// types are int32, uint32, int64, uint64, float32, float64, uintptr,
// unsafe.Pointer, and *T (an opaque native pointer). An omitted result is void.
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
	for _, field := range fn.Type.Params.List {
		t, err := scalarType(field.Type)
		if err != nil {
			return Declaration{}, err
		}
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for i := 0; i < count; i++ {
			d.Signature.Args = append(d.Signature.Args, t)
		}
	}
	if fn.Type.Results.NumFields() > 1 {
		return Declaration{}, fmt.Errorf("C ABI calls support at most one result")
	}
	if fn.Type.Results.NumFields() == 1 {
		d.Signature.Result, err = scalarType(fn.Type.Results.List[0].Type)
		if err != nil {
			return Declaration{}, err
		}
	}
	if err := d.Signature.Validate(); err != nil {
		return Declaration{}, err
	}
	return d, nil
}

func scalarType(expr ast.Expr) (abi.Type, error) {
	switch t := expr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "int32":
			return abi.I32, nil
		case "uint32":
			return abi.U32, nil
		case "int64":
			return abi.I64, nil
		case "uint64":
			return abi.U64, nil
		case "float32":
			return abi.F32, nil
		case "float64":
			return abi.F64, nil
		case "uintptr":
			if strconv.IntSize == 32 {
				return abi.U32, nil
			}
			return abi.U64, nil
		}
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok && pkg.Name == "unsafe" && t.Sel.Name == "Pointer" {
			return abi.Pointer, nil
		}
	case *ast.StarExpr:
		return abi.Pointer, nil
	case *ast.ParenExpr:
		return scalarType(t.X)
	}
	return abi.Void, fmt.Errorf("unsupported native scalar type; use explicit 32/64-bit integers, floats, or native pointers")
}

// Invocation includes literals in the form name(value:type, value:type)result.
type Invocation struct {
	Declaration
	Args []abi.Value
}

// ParseCall accepts whitespace or commas between typed arguments. Values are
// scalar literals, not Go expressions. Types and the optional result are
// validated by Parse, so both input forms share the same ABI rules.
func ParseCall(text string) (Invocation, error) {
	open := strings.IndexByte(text, '(')
	close, depth := -1, 0
	if open >= 0 {
		for i := open; i < len(text); i++ {
			switch text[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					close = i
				}
			}
			if close >= 0 {
				break
			}
		}
	}
	if open < 1 || close < open {
		return Invocation{}, fmt.Errorf("expected name(value:type, ...)result")
	}
	var values, types []string
	arguments := strings.TrimSpace(text[open+1 : close])
	if arguments != "" {
		parts := strings.Split(arguments, ":")
		if len(parts) < 2 {
			return Invocation{}, fmt.Errorf("each argument needs value:type")
		}
		values = append(values, strings.TrimSpace(parts[0]))
		for i := 1; i < len(parts)-1; i++ {
			part := strings.TrimSpace(parts[i])
			boundary := strings.LastIndexAny(part, ", \t\r\n")
			if boundary < 0 {
				return Invocation{}, fmt.Errorf("separate typed arguments with whitespace or commas")
			}
			typeText := strings.TrimSpace(part[:boundary])
			if part[boundary] != ',' {
				typeText = strings.TrimSpace(strings.TrimSuffix(typeText, ","))
			}
			types = append(types, typeText)
			values = append(values, strings.TrimSpace(part[boundary+1:]))
		}
		types = append(types, strings.TrimSpace(parts[len(parts)-1]))
	}
	d, err := Parse("func " + strings.TrimSpace(text[:open]) + "(" + strings.Join(types, ",") + ")" + strings.TrimSpace(text[close+1:]))
	if err != nil {
		return Invocation{}, err
	}
	if len(d.Signature.Args) != len(values) {
		return Invocation{}, fmt.Errorf("each argument needs one scalar type")
	}
	call := Invocation{Declaration: d}
	for i, text := range values {
		v, err := ParseValue(d.Signature.Args[i], text)
		if err != nil {
			return Invocation{}, fmt.Errorf("argument %d: %w", i+1, err)
		}
		call.Args = append(call.Args, v)
	}
	return call, nil
}
