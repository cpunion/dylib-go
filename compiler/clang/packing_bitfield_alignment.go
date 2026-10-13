package clang

import (
	"fmt"
	"strings"
)

// MS allocation units honor alignment at starts, and zero-width boundaries
// only immediately after a nonzero bitfield. Ask the producer about each active
// declaration in isolation; unrelated root alignment must not raise this value.
func (h *Header) bitfieldAlignmentProbes() (declarations, constants string) {
	if !h.Target.msLayout() {
		return "", ""
	}
	var types, values strings.Builder
	for i, r := range h.Records {
		if r.MaxFieldAlignment == 0 || !r.hasBitfields() {
			continue
		}
		fmt.Fprintf(&types, "#pragma pack(push,%d)\n", r.MaxFieldAlignment)
		members := recordMembers(r)
		starts := bitfieldUnitStarts(r, members)
		for position, member := range members {
			if !starts[position] {
				continue
			}
			kind, width, attributes := "", 0, ""
			key := ""
			if member.padding >= 0 {
				p := r.Padding[member.padding]
				kind, width, attributes = bitfieldBaseType(p.Type), p.Width, p.cAttributes()
				key = fmt.Sprintf("dylib_record_%d_paddingalign_%d", i, member.padding)
			} else {
				j := member.field
				kind, width, attributes = bitfieldBaseType(r.Description.Fields[j].Type.Type), r.bitfieldAt(j).Width, r.cFieldAttributes(j)
				key = fmt.Sprintf("dylib_record_%d_memberalign_%d", i, j)
			}
			name := key + "_type"
			packing := ""
			if r.Packed {
				packing = "__attribute__((packed)) "
			}
			fmt.Fprintf(&types, "typedef struct %s{\n", packing)
			if width == 0 {
				// A byte-sized predecessor activates the boundary without
				// contributing a larger alignment of its own.
				fmt.Fprint(&types, "unsigned char previous:1;\n")
			}
			fmt.Fprintf(&types, "%s :%d%s;\n} %s;\n", kind, width, attributes, name)
			fmt.Fprintf(&values, "%s=_Alignof(%s),\n", key, name)
		}
		fmt.Fprint(&types, "#pragma pack(pop)\n")
	}
	return types.String(), values.String()
}
