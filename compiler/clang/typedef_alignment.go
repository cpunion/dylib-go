package clang

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ramblelabs/dylib-go/abi"
)

// TypedefFieldAlignment preserves an aligned scalar/native-address, record or fixed-array
// alias and the compiler-observed placement of a named member. Windows
// compilers can ignore a lowered alias alignment differently for different types.
type TypedefFieldAlignment struct {
	Position int
	// ArrayDepth is zero for the scalar/address leaf, one for its innermost
	// array, and increases towards the outermost array. It does not alter size.
	ArrayDepth       int
	Aligned          uint64
	StorageAlignment uint64
}

// typedefAlignment follows written alias names, not desugared spellings. An
// outer aligned typedef replaces inherited alignment, including a decrease.
func (r *typeResolver) typedefAlignment(spelling string, depth int) (uint64, error) {
	fields, err := r.typedefAlignments(spelling, depth)
	if err != nil || len(fields) == 0 {
		return 0, err
	}
	return fields[len(fields)-1].Aligned, nil
}

// Array layers are counted from the leaf, so wrapping an alias in another array
// retains its alignment at the same layer. A new request replaces an inherited
// request only at that layer, including decreases.
func (r *typeResolver) typedefAlignments(spelling string, depth int) ([]TypedefFieldAlignment, error) {
	if depth > 16 {
		return nil, fmt.Errorf("C typedef nesting exceeds 16 levels")
	}
	spelling = unqualified(spelling)
	if open := strings.IndexByte(spelling, '['); open >= 0 && !strings.ContainsAny(spelling, "()") {
		spelling = strings.TrimSpace(spelling[:open])
	}
	node, ok := r.aliasNodes[spelling]
	if !ok {
		return nil, nil
	}
	inherited, err := r.typedefAlignments(node.Type.QualType, depth+1)
	if err != nil {
		return nil, err
	}
	var requested uint64
	arrayDepth := 0
	for _, child := range node.Inner {
		if !strings.HasSuffix(child.Kind, "Attr") {
			continue
		}
		if child.Kind != "AlignedAttr" {
			return nil, fmt.Errorf("unsupported typedef attribute %s on %s", child.Kind, spelling)
		}
		base, err := effectiveTypeNode(node)
		if err != nil {
			return nil, err
		}
		arrayDepth = 0
		for base.Kind == "ConstantArrayType" && len(base.Inner) == 1 {
			arrayDepth++
			if arrayDepth > 8 {
				return nil, fmt.Errorf("C array nesting exceeds 8 levels")
			}
			base, err = effectiveTypeNode(base.Inner[0])
			if err != nil {
				return nil, err
			}
		}
		if base.Kind != "BuiltinType" && base.Kind != "EnumType" && base.Kind != "PointerType" && base.Kind != "RecordType" {
			return nil, fmt.Errorf("unsupported typedef attribute AlignedAttr on %s: requires a scalar, pointer, record or fixed array", spelling)
		}
		alignment, err := r.recordAlignment(child)
		if err != nil {
			return nil, fmt.Errorf("typedef %s: %w", spelling, err)
		}
		if alignment > requested {
			requested = alignment
		}
	}
	if requested != 0 {
		for i := range inherited {
			if inherited[i].ArrayDepth == arrayDepth {
				inherited[i].Aligned = requested
				return inherited, nil
			}
		}
		inherited = append(inherited, TypedefFieldAlignment{ArrayDepth: arrayDepth, Aligned: requested})
		sort.Slice(inherited, func(i, j int) bool { return inherited[i].ArrayDepth < inherited[j].ArrayDepth })
	}
	return inherited, nil
}

func (r Record) fieldTypeAlignment(position int) uint64 {
	for _, field := range r.FieldTypeAlignments {
		if field.Position == position {
			return field.Aligned
		}
	}
	return 0
}

func validateFieldTypeAlignments(r Record, complete bool) error {
	if complete && len(r.FieldTypeAlignments) != 0 && len(r.Layout.Offsets) != len(r.Description.Fields) {
		return fmt.Errorf("clang: missing typedef member offsets in %s", r.Name)
	}
	previous, previousDepth := -1, -1
	var previousStorage uint64
	for _, field := range r.FieldTypeAlignments {
		if field.Position < 0 || field.Position < previous || field.Position == previous && field.ArrayDepth <= previousDepth || field.Position >= len(r.Description.Fields) || field.ArrayDepth < 0 || field.Aligned == 0 || !validAlignment(field.Aligned) || r.bitfieldAt(field.Position) != nil {
			return fmt.Errorf("clang: invalid typedef field alignment in %s", r.Name)
		}
		d := r.Description.Fields[field.Position].Type
		arrayDepth := 0
		for d.Type == abi.Array {
			if d.Elem == nil {
				return fmt.Errorf("clang: missing typedef array element in %s", r.Name)
			}
			arrayDepth++
			d = *d.Elem
		}
		if field.ArrayDepth > arrayDepth || d.Type == abi.Void || d.Elem != nil {
			return fmt.Errorf("clang: typedef field alignment requires a scalar/native-address/record leaf or fixed array in %s", r.Name)
		}
		if !validAlignment(field.StorageAlignment) || field.Position == previous && field.StorageAlignment != previousStorage || complete && (field.StorageAlignment == 0 || field.StorageAlignment > r.Layout.Alignment || r.Layout.Offsets[field.Position]%field.StorageAlignment != 0) {
			return fmt.Errorf("clang: invalid typedef member storage alignment in %s", r.Name)
		}
		previous, previousDepth, previousStorage = field.Position, field.ArrayDepth, field.StorageAlignment
	}
	return nil
}
