package signature

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"strconv"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// ParseTypedValue accepts struct and array literals in a known C layout.
// Pointer literals &VALUE (or an aggregate literal for its pointer) become
// temporary native copies, without passing Go memory to the native function.
func ParseTypedValue(d abi.TypeDesc, text string) (abi.Value, error) {
	if err := d.Validate(); err != nil {
		return abi.Value{}, err
	}
	text = strings.TrimSpace(text)
	if d.Type != abi.Struct && d.Type != abi.Array && !(d.Type == abi.Pointer && (strings.HasPrefix(text, "&") || strings.HasPrefix(text, "{"))) {
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
		return abi.Value{}, fmt.Errorf("expected an aggregate literal {...}")
	}
	expr, err := parser.ParseExpr(expandPointeeLiterals("_native" + text))
	if err != nil {
		return abi.Value{}, err
	}
	return parseComposite(d, expr)
}

// Go permits elided nested aggregate literals, but &{...} needs a type token.
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
	if d.Type != abi.Struct && d.Type != abi.Array {
		var buf bytes.Buffer
		if err := format.Node(&buf, token.NewFileSet(), expr); err != nil {
			return abi.Value{}, err
		}
		return ParseValue(d.Type, buf.String())
	}
	composite, ok := expr.(*ast.CompositeLit)
	if !ok {
		return abi.Value{}, fmt.Errorf("expected an aggregate literal")
	}
	if composite.Type != nil {
		name, ok := composite.Type.(*ast.Ident)
		if !ok || name.Name != "_native" {
			return abi.Value{}, fmt.Errorf("aggregate literals must omit their type name")
		}
	}
	if d.Type == abi.Array {
		return parseArrayComposite(d, composite)
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

func parseArrayComposite(d abi.TypeDesc, composite *ast.CompositeLit) (abi.Value, error) {
	v := abi.Zero(d)
	seen := make(map[int]bool)
	index := 0
	for _, e := range composite.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			key, ok := kv.Key.(*ast.BasicLit)
			if !ok || key.Kind != token.INT {
				return abi.Value{}, fmt.Errorf("array index must be a nonnegative integer literal")
			}
			n, err := strconv.ParseUint(key.Value, 0, 32)
			if err != nil || n >= uint64(d.Len) {
				return abi.Value{}, fmt.Errorf("array index outside length %d", d.Len)
			}
			index, e = int(n), kv.Value
		}
		if index >= d.Len || seen[index] {
			return abi.Value{}, fmt.Errorf("too many or duplicate array elements")
		}
		seen[index] = true
		element, err := parseComposite(*d.Elem, e)
		if err != nil {
			return abi.Value{}, fmt.Errorf("array element %d: %w", index, err)
		}
		v.Aggregate.Fields[index] = element
		index++
	}
	return v, nil
}
