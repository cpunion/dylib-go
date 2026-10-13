package clang

import (
	"bytes"
	"fmt"
)

// A MinGW outer packing pragma would suppress MSVC's required member/unit
// alignment. Keep native types and full allocation units, using byte-aligned
// anonymous types with alignment on the member, which does not round unit size.
func (t *cgoTypes) writePragmaBitfields(output *bytes.Buffer, index int) error {
	r := t.records[index]
	if err := validateMemberAlignments(r, true); err != nil {
		return err
	}
	members := recordMembers(r)
	units := bitfieldUnits(r, members)
	starts := bitfieldUnitStarts(r, members)
	for position, member := range members {
		alignment := uint64(0)
		if member.padding >= 0 {
			alignment = r.Padding[member.padding].StorageAlignment
		} else if r.bitfieldAt(member.field) != nil {
			alignment = r.MemberAlignments[member.field]
		} else {
			continue
		}
		if starts[position] != (alignment != 0) {
			return fmt.Errorf("clang: missing or inconsistent MS allocation alignment in %s", r.Name)
		}
	}
	fmt.Fprintf(output, "typedef struct %s{\n", r.cAttributes())
	for position, member := range members {
		for _, unit := range units {
			if unit.first == position {
				alignment := uint64(0)
				if member.padding >= 0 {
					alignment = r.Padding[member.padding].StorageAlignment
				} else {
					alignment = r.MemberAlignments[member.field]
				}
				fmt.Fprintf(output, "#pragma pack(push,1)\n_Alignas(%d) struct {\n", alignment)
			}
		}
		if member.padding >= 0 {
			p := r.Padding[member.padding]
			if p.Width == 0 && p.StorageAlignment != 0 {
				fmt.Fprintf(output, "_Alignas(%d) unsigned char boundary%d[0];\n", p.StorageAlignment, member.padding)
			}
			// Keep zero-width declarations for the type ABI. An anonymous
			// unit/ordinary boundary precedes them, so MS layout ignores
			// their requests; the measured boundary above remains effective.
			fmt.Fprintf(output, "%s :%d;\n", bitfieldBaseType(p.Type), p.Width)
		} else {
			j := member.field
			declaration, err := t.cField(r.Description.Fields[j].Type, fmt.Sprintf("f%d", j))
			if err != nil {
				return fmt.Errorf("clang: %s: %w", r.Name, err)
			}
			if bit := r.bitfieldAt(j); bit != nil {
				fmt.Fprintf(output, "%s:%d;\n", declaration, bit.Width)
			} else {
				fmt.Fprintf(output, "%s%s;\n", declaration, storageFieldAttributes(r.Description.Fields[j].Type, r.MemberAlignments[j]))
			}
		}
		for _, unit := range units {
			if unit.last == position {
				fmt.Fprint(output, "};\n#pragma pack(pop)\n")
			}
		}
	}
	fmt.Fprintf(output, "} %s;\n", t.cRecordName(index))
	return nil
}

// GNU zero-width alignment bypasses the packing cap in both compilers, but GCC
// also raises the record alignment. Unnamed padding preserves Clang's observed
// boundary without adding value fields to the SysV argument classification.
func writeGNUPragmaBoundary(output *bytes.Buffer, gap uint64) {
	fmt.Fprint(output, "unsigned char :0;\n")
	for gap >= 8 {
		fmt.Fprint(output, "unsigned long long :64 __attribute__((packed));\n")
		gap -= 8
	}
	for ; gap != 0; gap-- {
		fmt.Fprint(output, "unsigned char :8;\n")
	}
	fmt.Fprint(output, "unsigned char :0;\n")
}
