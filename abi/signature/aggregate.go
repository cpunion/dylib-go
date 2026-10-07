package signature

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// ParseTypedValue accepts ordinary struct literals in the context of a known
// C layout. Pointer literals &VALUE (or a struct literal for *struct) become
// temporary native copies, without passing Go memory to the native function.
func ParseTypedValue(d abi.TypeDesc, text string) (abi.Value, error) {
	if err := d.Validate(); err != nil {
		return abi.Value{}, err
	}
	text = strings.TrimSpace(text)
	if d.Type != abi.Struct && !(d.Type == abi.Pointer && (strings.HasPrefix(text, "&") || strings.HasPrefix(text, "{"))) {
		return ParseValue(d.Type, text)
	}
	if d.Type == abi.Pointer {
		if d.Elem == nil {
			return abi.Value{}, fmt.Errorf("temporary pointer literal requires a known pointee type")
		}
		v, err := ParseTypedValue(*d.Elem, strings.TrimSpace(strings.TrimPrefix(text, "&")))
		if err != nil {
			return abi.Value{}, err
		}
		return abi.AddressOf(&v), nil
	}
	if !strings.HasPrefix(text, "{") {
		return abi.Value{}, fmt.Errorf("expected a struct literal {...}")
	}
	expr, err := parser.ParseExpr(expandPointeeLiterals("_native" + text))
	if err != nil {
		return abi.Value{}, err
	}
	return parseComposite(d, expr)
}

// Go permits elided nested struct literals, but &{...} needs a type token.
// Insert a placeholder only at that token boundary; the descriptor supplies
// the actual type, so no expression is evaluated or identifier resolved.
func expandPointeeLiterals(text string) string {
	file := token.NewFileSet().AddFile("literal", -1, len(text))
	var lexer scanner.Scanner
	lexer.Init(file, []byte(text), nil, 0)
	var out strings.Builder
	previous := token.ILLEGAL
	start := 0
	for {
		pos, kind, _ := lexer.Scan()
		if kind == token.EOF {
			break
		}
		if previous == token.AND && kind == token.LBRACE {
			offset := file.Offset(pos)
			out.WriteString(text[start:offset])
			out.WriteString("_native")
			start = offset
		}
		previous = kind
	}
	out.WriteString(text[start:])
	return out.String()
}

func parseComposite(d abi.TypeDesc, expr ast.Expr) (abi.Value, error) {
	if d.Type == abi.Pointer {
		if u, ok := expr.(*ast.UnaryExpr); ok && u.Op == token.AND {
			if d.Elem == nil {
				return abi.Value{}, fmt.Errorf("unknown pointee type")
			}
			v, err := parseComposite(*d.Elem, u.X)
			if err != nil {
				return abi.Value{}, err
			}
			return abi.AddressOf(&v), nil
		}
		if _, literal := expr.(*ast.CompositeLit); literal && d.Elem != nil {
			v, err := parseComposite(*d.Elem, expr)
			if err != nil {
				return abi.Value{}, err
			}
			return abi.AddressOf(&v), nil
		}
	}
	if d.Type != abi.Struct {
		var buf bytes.Buffer
		if err := format.Node(&buf, token.NewFileSet(), expr); err != nil {
			return abi.Value{}, err
		}
		return ParseValue(d.Type, buf.String())
	}
	composite, ok := expr.(*ast.CompositeLit)
	if !ok {
		return abi.Value{}, fmt.Errorf("expected a struct literal")
	}
	if composite.Type != nil {
		name, ok := composite.Type.(*ast.Ident)
		if !ok || name.Name != "_native" {
			return abi.Value{}, fmt.Errorf("struct literals must omit their type name")
		}
	}
	v := abi.Zero(d)
	seen := make(map[int]bool)
	keyed := false
	for i, e := range composite.Elts {
		index := i
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			if i > 0 && !keyed {
				return abi.Value{}, fmt.Errorf("cannot mix named and positional fields")
			}
			keyed = true
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				return abi.Value{}, fmt.Errorf("struct field key must be a name")
			}
			index = -1
			for j, f := range d.Fields {
				if f.Name == key.Name {
					index = j
					break
				}
			}
			if index < 0 {
				return abi.Value{}, fmt.Errorf("unknown struct field %q", key.Name)
			}
			e = kv.Value
		} else if keyed {
			return abi.Value{}, fmt.Errorf("cannot mix named and positional fields")
		}
		if index >= len(d.Fields) || seen[index] {
			return abi.Value{}, fmt.Errorf("too many or duplicate struct fields")
		}
		seen[index] = true
		field, err := parseComposite(d.Fields[index].Type, e)
		if err != nil {
			return abi.Value{}, fmt.Errorf("field %q: %w", d.Fields[index].Name, err)
		}
		v.Aggregate.Fields[index] = field
	}
	if len(composite.Elts) > 0 && !keyed && len(composite.Elts) != len(d.Fields) {
		return abi.Value{}, fmt.Errorf("positional literal needs all struct fields")
	}
	return v, nil
}
