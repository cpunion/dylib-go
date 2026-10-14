package clang

import (
	"bytes"
	"fmt"

	"github.com/cpunion/dylib-go/abi"
)

// MSVC i386 returns small single-float aggregates in integer registers; MinGW
// uses x87. A private overlapping integer member disables MinGW's single-float
// shortcut without changing the logical field, offsets, size or alignment.
func (t *cgoTypes) writeNativeField(output *bytes.Buffer, r Record, index int, declaration, attributes string) {
	if bit := r.bitfieldAt(index); bit != nil && bit.TypeAlignment != 0 {
		aligned := bit.StorageAlignment
		packed := r.fieldPacked(index)
		member := r.fieldAlignment(index)
		if t.gnuPacking {
			natural := uint64(bitfieldTypeBits(r.Description.Fields[index].Type.Type) / 8)
			if natural == 0 {
				natural = 1
			}
			packed = packed || bit.TypeAlignment < natural
			if r.MaxFieldAlignment != 0 && member > r.MaxFieldAlignment {
				member = 0
			}
		}
		if !t.msLayout && member > aligned {
			aligned = member
		}
		attributes = fieldCAttributes(packed, aligned)
	}
	// Normalize SysV typedef members to canonical scalar fields with measured
	// packing/alignment. Parse separately verifies that the producer follows
	// Clang 19-compatible lowering; layouts alone cannot distinguish Clang 18.
	if (t.windows || t.gnuAMD64) && r.fieldTypeAlignment(index) != 0 {
		for _, field := range r.FieldTypeAlignments {
			if field.Position == index {
				attributes = storageFieldAttributes(r.Description.Fields[index].Type, field.StorageAlignment)
				break
			}
		}
	}
	if index == 0 && t.msFloatReturn(r) {
		fmt.Fprintf(output, "#if defined(_WIN32) && defined(__i386__) && !defined(_MSC_VER)\nunion {%s; uint%d_t return_bits;}%s;\n#else\n%s%s;\n#endif\n", declaration, 8*r.Layout.Size, attributes, declaration, attributes)
		return
	}
	fmt.Fprintf(output, "%s%s;\n", declaration, attributes)
}

func (t *cgoTypes) msFloatReturn(r Record) bool {
	if !t.ms386 || len(r.Description.Fields) != 1 || r.Layout.Offsets[0] != 0 || r.Layout.Size != 4 && r.Layout.Size != 8 {
		return false
	}
	for _, padding := range r.Padding {
		if padding.Width != 0 {
			return false
		}
	}
	d := r.Description.Fields[0].Type
	for d.Type == abi.Array && d.Len == 1 {
		d = *d.Elem
	}
	return d.Type == abi.F32 || d.Type == abi.F64
}
