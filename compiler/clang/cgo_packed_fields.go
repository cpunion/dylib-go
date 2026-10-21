package clang

import (
	"bytes"
	"fmt"

	"github.com/ramblelabs/dylib-go/abi"
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

func (m recordMember) packed(r Record) bool {
	if m.padding >= 0 {
		return r.Padding[m.padding].Packed
	}
	return r.fieldPacked(m.field)
}

type bitfieldUnit struct {
	first, last int // Indexes in the complete declaration stream.
}

// MinGW ignores member packed attributes on MS-layout bitfields. An anonymous
// record with a scoped packing pragma preserves one producer allocation unit
// without changing the natural alignment of neighboring ordinary members.
// MS layout groups successive equal-sized base types while the widths fit;
// zero-width fields and ordinary members always end an allocation unit.
func packedBitfieldUnits(r Record, members []recordMember) []bitfieldUnit {
	if r.Packed {
		return nil // Record-wide packing already has its own bridge.
	}
	var packed []bitfieldUnit
	for _, unit := range bitfieldUnits(r, members) {
		if members[unit.first].packed(r) {
			packed = append(packed, unit)
		}
	}
	return packed
}

func bitfieldUnits(r Record, members []recordMember) []bitfieldUnit {
	var units []bitfieldUnit
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
		units = append(units, bitfieldUnit{first, last})
		first = last + 1
	}
	return units
}

// In MS layout, nonzero fields honor alignment only at unit starts. A zero-width
// declaration only honors it immediately after a nonzero bitfield.
func bitfieldUnitStarts(r Record, members []recordMember) map[int]bool {
	starts := make(map[int]bool)
	for _, unit := range bitfieldUnits(r, members) {
		starts[unit.first] = true
	}
	for i, member := range members {
		if member.padding >= 0 && r.Padding[member.padding].Width == 0 && i > 0 {
			_, width := members[i-1].bitfield(r)
			starts[i] = width != 0
		}
	}
	return starts
}

// A zero-width field after a wrapped unit now follows an ordinary anonymous
// member, so MS layout would ignore it. MinGW also ignores the packed attribute
// on zero-width bitfields. A zero-length array restores the effective boundary
// without allocating bytes. Leading/consecutive zero-width fields stay ignored.
func writePackedUnitBoundary(output *bytes.Buffer, p BitfieldPadding, index int) {
	fmt.Fprintf(output, "#if defined(_WIN32) && !defined(_MSC_VER)\n%s boundary%d[0]%s;\n#else\n%s :0%s;\n#endif\n", bitfieldBaseType(p.Type), index, p.cAttributes(), bitfieldBaseType(p.Type), p.cAttributes())
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
