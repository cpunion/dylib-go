package clang

import (
	"bytes"
	"fmt"

	"github.com/cpunion/dylib-go/abi"
)

// recordMember retains the full declaration order, including unnamed fields.
// Exactly one index is nonnegative.
type recordMember struct {
	field, padding int
}

func recordMembers(r Record) []recordMember {
	members := make([]recordMember, 0, len(r.Description.Fields)+len(r.Padding))
	padding := 0
	for field := 0; field <= len(r.Description.Fields); field++ {
		for padding < len(r.Padding) && r.Padding[padding].Before == field {
			members = append(members, recordMember{-1, padding})
			padding++
		}
		if field < len(r.Description.Fields) {
			members = append(members, recordMember{field, -1})
		}
	}
	return members
}

func (m recordMember) bitfield(r Record) (capacity, width int) {
	var kind abi.Type
	if m.padding >= 0 {
		p := r.Padding[m.padding]
		kind, width = p.Type, p.Width
	} else if bit := r.bitfieldAt(m.field); bit != nil {
		kind, width = r.Description.Fields[m.field].Type.Type, bit.Width
	} else {
		return 0, 0
	}
	capacity = bitfieldTypeBits(kind)
	if kind == abi.Bool {
		capacity = 8 // Boolean bitfields still own a byte allocation unit.
	}
	return
}

type packedBitfieldUnit struct {
	first, last int // Indexes in the complete declaration stream.
}

// MinGW ignores member packed attributes on MS-layout bitfields. An anonymous
// record with a scoped packing pragma preserves one producer allocation unit
// without changing the natural alignment of neighboring ordinary members.
// MS layout groups successive equal-sized base types while the widths fit;
// zero-width fields and ordinary members always end an allocation unit.
func packedBitfieldUnits(r Record, members []recordMember) []packedBitfieldUnit {
	if r.Packed {
		return nil // Record-wide packing already has its own bridge.
	}
	var units []packedBitfieldUnit
	for first := 0; first < len(members); {
		capacity, used := members[first].bitfield(r)
		if used == 0 {
			first++
			continue
		}
		last := first
		for next := first + 1; next < len(members); next++ {
			size, width := members[next].bitfield(r)
			if width == 0 || size != capacity || used+width > capacity {
				break
			}
			used += width
			last = next
		}
		if field := members[first].field; field >= 0 && r.fieldPacked(field) {
			units = append(units, packedBitfieldUnit{first, last})
		}
		first = last + 1
	}
	return units
}

// A zero-width field after a wrapped unit now follows an ordinary anonymous
// member, so MS layout would ignore it. A zero-length array restores the same
// natural alignment boundary without allocating bytes. Only that immediate
// boundary is replaced; leading/consecutive zero-width fields remain ignored.
func writePackedUnitBoundary(output *bytes.Buffer, p BitfieldPadding, index int) {
	fmt.Fprintf(output, "#if defined(_WIN32) && !defined(_MSC_VER)\n%s boundary%d[0];\n#else\n%s :0;\n#endif\n", bitfieldBaseType(p.Type), index, bitfieldBaseType(p.Type))
}

func beginPackedBitfieldUnit(output *bytes.Buffer, alignment uint64) {
	fmt.Fprint(output, "#if defined(_WIN32) && !defined(_MSC_VER)\n#pragma pack(push,1)\n")
	if alignment != 0 {
		// Align the anonymous member, not its type: type alignment would add
		// tail padding to the allocation unit. A trailing GNU attribute is
		// ignored here by MinGW, while C11 field alignment is preserved.
		fmt.Fprintf(output, "_Alignas(%d) ", alignment)
	}
	fmt.Fprint(output, "struct {\n#endif\n")
}

func endPackedBitfieldUnit(output *bytes.Buffer) {
	fmt.Fprint(output, "#if defined(_WIN32) && !defined(_MSC_VER)\n};\n#pragma pack(pop)\n#endif\n")
}
