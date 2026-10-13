package clang

import (
	"fmt"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// TypedefFieldAlignment preserves an aligned scalar/native-address alias and the
// compiler-observed placement of a named member (or its fixed array). Windows
// compilers can ignore a lowered alias alignment differently for different types.
type TypedefFieldAlignment struct {
	Position         int
	Aligned          uint64
	StorageAlignment uint64
}

// typedefAlignment follows written alias names, not desugared spellings. An
// outer aligned typedef replaces inherited alignment, including a decrease.
// Record/array typedef attributes need separate native type identity and remain
// unsupported. Arrays of aligned scalars retain the scalar alias at each leaf.
func (r *typeResolver) typedefAlignment(spelling string, depth int) (uint64, error) {
	if depth > 16 {
		return 0, fmt.Errorf("C typedef nesting exceeds 16 levels")
	}
	spelling = unqualified(spelling)
	if open := strings.IndexByte(spelling, '['); open >= 0 && !strings.ContainsAny(spelling, "()") {
		spelling = strings.TrimSpace(spelling[:open])
	}
	node, ok := r.aliasNodes[spelling]
	if !ok {
		return 0, nil
	}
	inherited, err := r.typedefAlignment(node.Type.QualType, depth+1)
	if err != nil {
		return 0, err
	}
	var requested uint64
	for _, child := range node.Inner {
		if !strings.HasSuffix(child.Kind, "Attr") {
			continue
		}
		if child.Kind != "AlignedAttr" {
			return 0, fmt.Errorf("unsupported typedef attribute %s on %s", child.Kind, spelling)
		}
		base, err := effectiveTypeNode(node)
		if err != nil {
			return 0, err
		}
		if base.Kind != "BuiltinType" && base.Kind != "EnumType" && base.Kind != "PointerType" {
			return 0, fmt.Errorf("unsupported typedef attribute AlignedAttr on %s: requires a scalar or pointer", spelling)
		}
		alignment, err := r.recordAlignment(child)
		if err != nil {
			return 0, fmt.Errorf("typedef %s: %w", spelling, err)
		}
		if alignment > requested {
			requested = alignment
		}
	}
	if requested != 0 {
		return requested, nil
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
	previous := -1
	for _, field := range r.FieldTypeAlignments {
		if field.Position <= previous || field.Position >= len(r.Description.Fields) || field.Aligned == 0 || !validAlignment(field.Aligned) || r.bitfieldAt(field.Position) != nil {
			return fmt.Errorf("clang: invalid typedef field alignment in %s", r.Name)
		}
		d := r.Description.Fields[field.Position].Type
		for d.Type == abi.Array {
			d = *d.Elem
		}
		if d.Type == abi.Struct || d.Type == abi.Void || d.Elem != nil {
			return fmt.Errorf("clang: typedef field alignment requires scalar/native-address leaves in %s", r.Name)
		}
		if !validAlignment(field.StorageAlignment) || complete && (field.StorageAlignment == 0 || field.StorageAlignment > r.Layout.Alignment || r.Layout.Offsets[field.Position]%field.StorageAlignment != 0) {
			return fmt.Errorf("clang: invalid typedef member storage alignment in %s", r.Name)
		}
		previous = field.Position
	}
	return nil
}
