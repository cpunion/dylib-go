package clang

import (
	"bytes"
	"fmt"

	"github.com/cpunion/dylib-go/abi"
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
	fmt.Fprintf(output, "typedef struct %s{\n", t.nativeAttributes(r))
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
			declaration, err := t.cNativeField(r, j)
			if err != nil {
				return fmt.Errorf("clang: %s: %w", r.Name, err)
			}
			if bit := r.bitfieldAt(j); bit != nil {
				fmt.Fprintf(output, "%s:%d;\n", declaration, bit.Width)
			} else {
				t.writeNativeField(output, r, j, declaration, storageFieldAttributes(r.Description.Fields[j].Type, r.MemberAlignments[j]))
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
// boundary. Small SysV register arguments need a separate implicit-padding
// facade because GCC classifies these nonzero declarations as integer values.
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

// A trailing gap must remain padding in the SysV ABI.
// GCC classifies unnamed nonzero bitfields as integer values. Round the body
// with alignment instead, then lower its typedef alignment to the native value.
// This changes size without introducing an extra argument/result register.
func gnuImplicitTailPadding(r Record) bool {
	if r.MaxFieldAlignment == 0 || r.Layout.Size > 16 || r.Layout.Size&(r.Layout.Size-1) != 0 {
		return false
	}
	found := false
	for _, p := range r.Padding {
		if p.Before == len(r.Description.Fields) {
			if p.Width != 0 {
				return false
			}
			found = found || p.Aligned > r.MaxFieldAlignment && p.Gap != 0 && p.Offset == r.Layout.Size
		}
	}
	return found
}

// Small interior gaps also need implicit padding to preserve SysV registers.
// Wrap the preceding fields in anonymous aligned records: their rounded size
// supplies each gap, while the outer pragma keeps the original base alignment.
func (t *cgoTypes) gnuPrefixPadding(r Record) map[int]bool {
	if !t.gnuAMD64 || r.MaxFieldAlignment == 0 || r.Layout.Size > 16 || !t.sysVAlignedRecord(r, 0) {
		return nil
	}
	for _, padding := range r.Padding {
		if padding.Width != 0 {
			// Original unnamed values can already affect GCC's register
			// classification; this facade only qualifies zero-width gaps.
			return nil
		}
	}
	var result map[int]bool
	for index, padding := range r.Padding {
		if padding.Aligned <= r.MaxFieldAlignment || padding.Aligned <= r.Layout.Alignment || padding.Gap == 0 || padding.Before == 0 || padding.Before == len(r.Description.Fields) {
			continue
		}
		if padding.Offset == 0 || padding.Offset >= r.Layout.Size || padding.Offset&(padding.Offset-1) != 0 {
			continue
		}
		if result == nil {
			result = make(map[int]bool)
		}
		result[index] = true
	}
	return result
}

// Unaligned ordinary fields already force SysV memory arguments. Changing body
// alignment there would change the required argument/result storage alignment.
func (t *cgoTypes) sysVAlignedRecord(r Record, base uint64) bool {
	for i, field := range r.Description.Fields {
		if r.bitfieldAt(i) == nil && !t.sysVAlignedType(field.Type, base+r.Layout.Offsets[i]) {
			return false
		}
	}
	return true
}

func (t *cgoTypes) sysVAlignedType(d abi.TypeDesc, offset uint64) bool {
	if d.Type == abi.Struct {
		i, err := t.recordIndex(d)
		return err == nil && t.sysVAlignedRecord(t.records[i], offset)
	}
	if d.Type == abi.Array {
		// This helper only qualifies records of at most sixteen bytes.
		if d.Len > 16 {
			return false
		}
		size := t.sysVTypeSize(*d.Elem)
		for i := 0; i < d.Len; i++ {
			if !t.sysVAlignedType(*d.Elem, offset+uint64(i)*size) {
				return false
			}
		}
		return size != 0
	}
	size := t.sysVTypeSize(d)
	return size != 0 && offset%size == 0
}

func (t *cgoTypes) sysVTypeSize(d abi.TypeDesc) uint64 {
	switch d.Type {
	case abi.Bool, abi.I8, abi.U8:
		return 1
	case abi.I16, abi.U16:
		return 2
	case abi.I32, abi.U32, abi.F32:
		return 4
	case abi.I64, abi.U64, abi.F64, abi.Pointer:
		return 8
	case abi.Array:
		return uint64(d.Len) * t.sysVTypeSize(*d.Elem)
	case abi.Struct:
		i, err := t.recordIndex(d)
		if err == nil {
			return t.records[i].Layout.Size
		}
	}
	return 0
}
