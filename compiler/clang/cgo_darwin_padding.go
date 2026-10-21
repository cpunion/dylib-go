package clang

import (
	"fmt"

	"github.com/ramblelabs/dylib-go/abi"
)

// GCC counts plain unnamed units toward Darwin ARM64 record alignment; Clang
// does not. Anonymous byte fields preserve the measured gaps without raising
// alignment or making a homogeneous floating aggregate. This is not a SysV
// AMD64 facade: there, the same bytes can introduce incompatible integer lanes.
// Only the private C declaration changes; portable metadata keeps the original.
func (t *cgoTypes) darwinPaddingRecord(r Record) (Record, error) {
	if !t.darwinARM64 || len(r.Padding) == 0 || len(r.Bitfields) != 0 ||
		r.Packed || r.MaxFieldAlignment != 0 || r.Aligned != 0 ||
		len(r.PackedFields) != 0 || len(r.FieldAlignments) != 0 ||
		len(r.FieldTypeAlignments) != 0 || len(r.TypedefAlignments) != 0 ||
		r.ArgumentAlignment != 0 {
		return r, nil
	}
	for _, p := range r.Padding {
		if p.Width == 0 || p.Packed || p.Aligned != 0 || p.TypeAlignment != 0 {
			return r, nil
		}
	}
	var cursor uint64
	var padding []BitfieldPadding
	fill := func(before int, end uint64) {
		for ; cursor < end; cursor++ {
			padding = append(padding, BitfieldPadding{Before: before, Type: abi.U8, Width: 8})
		}
	}
	for i, field := range r.Description.Fields {
		dimensions, leaf := arrayDimensions(field.Type)
		size, err := t.arrayLeafSize(leaf)
		if err != nil {
			return r, err
		}
		for _, length := range dimensions {
			if size == 0 || uint64(length) > r.Layout.Size/size {
				return r, fmt.Errorf("clang: invalid Darwin unnamed-field array extent in %s", r.Name)
			}
			size *= uint64(length)
		}
		offset := r.Layout.Offsets[i]
		if offset < cursor || offset > r.Layout.Size || size > r.Layout.Size-offset {
			return r, fmt.Errorf("clang: invalid Darwin unnamed-field extent in %s", r.Name)
		}
		fill(i, offset)
		cursor = offset + size
	}
	fill(len(r.Description.Fields), r.Layout.Size)
	r.Padding, r.Aligned = padding, r.Layout.Alignment
	return r, nil
}
