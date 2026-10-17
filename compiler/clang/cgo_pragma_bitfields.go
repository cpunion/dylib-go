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
	if r.Layout.Size > 16 || r.Layout.Size&(r.Layout.Size-1) != 0 {
		return false
	}
	found := false
	for _, p := range r.Padding {
		if p.Width != 0 && p.Before != len(r.Description.Fields) {
			return false
		}
		if p.Before == len(r.Description.Fields) {
			if p.Width != 0 {
				found = found || len(r.Bitfields) == 0
				continue
			}
			found = found || (p.TypeAlignment != 0 || r.MaxFieldAlignment != 0 && p.Aligned > r.MaxFieldAlignment) && p.Gap != 0 && p.Offset == r.Layout.Size
		}
	}
	return found
}

// Small interior gaps also need implicit padding to preserve SysV registers.
// Wrap the preceding fields in anonymous aligned records: their rounded size
// supplies each gap, while the outer pragma keeps the original base alignment.
func (t *cgoTypes) gnuPrefixPadding(r Record) map[int]uint64 {
	if !t.gnuPacking {
		return nil
	}
	var result map[int]uint64
	for index, padding := range r.Padding {
		if r.gnuPlainZeroBoundary(padding) && uint64(bitfieldTypeBits(padding.Type)/8) > r.Layout.Alignment {
			// Round only the preceding prefix, retaining the observed root
			// alignment and implicit padding on every GNU target. i386's
			// long-long boundary can be four bytes despite its eight-byte size.
			alignment := min(uint64(bitfieldTypeBits(padding.Type)/8), padding.Offset&-padding.Offset)
			if padding.Before != 0 && alignment != 0 && padding.Gap < alignment {
				if t.gnuAMD64 && r.Layout.Size <= 16 && t.sysVAlignedRecord(r, 0) {
					unnamedValues := false
					for _, prior := range r.Padding[:index] {
						unnamedValues = unnamedValues || prior.Width != 0
					}
					if unnamedValues && !t.sysVIntegerPaddingCompatible(r, padding.Offset) {
						continue
					}
				}
				if result == nil {
					result = make(map[int]uint64)
				}
				result[index] = alignment
			}
			continue
		}
		if !t.gnuAMD64 || r.Layout.Size > 16 || !t.sysVAlignedRecord(r, 0) {
			continue
		}
		if padding.Width != 0 && (padding.TypeAlignment == 0 || hasFloatingFields(r.Description)) {
			continue
		}
		priorValues, laterValues := false, false
		for other, prior := range r.Padding {
			if prior.Width != 0 {
				if other < index {
					priorValues = true
				} else if padding.Width == 0 {
					laterValues = true
				}
			}
		}
		// Moving original unnamed values into an aligned prefix is safe
		// only when each affected eightbyte already has an integer class.
		// Values after the boundary need separate qualification.
		if laterValues || priorValues && (padding.Width != 0 || !t.sysVIntegerPaddingCompatible(r, padding.Offset)) {
			continue
		}
		if padding.TypeAlignment == 0 && padding.Aligned <= r.MaxFieldAlignment || padding.requestedAlignment() <= r.Layout.Alignment || padding.Gap == 0 || padding.Before == 0 || padding.Before == len(r.Description.Fields) {
			continue
		}
		if padding.Offset == 0 || padding.Offset >= r.Layout.Size || padding.Offset&(padding.Offset-1) != 0 {
			continue
		}
		if result == nil {
			result = make(map[int]uint64)
		}
		result[index] = padding.Offset
	}
	return result
}

// Unnamed fields inside a prefix cannot add a SysV integer class when each
// eight-byte chunk touched by that prefix already has a named integer value.
// Composite/floating members do not establish that class in this proof.
func (t *cgoTypes) sysVIntegerPaddingCompatible(r Record, extent uint64) bool {
	var chunks uint64
	for i, field := range r.Description.Fields {
		if field.Type.Type == abi.Struct || field.Type.Type == abi.Array || field.Type.Type == abi.F32 || field.Type.Type == abi.F64 {
			continue
		}
		size := t.sysVTypeSize(field.Type)
		if bit := r.bitfieldAt(i); bit != nil {
			size = uint64(len(bit.Mask))
		}
		for position := r.Layout.Offsets[i]; position < r.Layout.Offsets[i]+size; position++ {
			chunks |= 1 << (position / 8)
		}
	}
	required := (uint64(1) << ((extent + 7) / 8)) - 1
	return chunks&required == required
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

// Integer-only SysV records already classify occupied padding lanes as integer.
// Floating fields need an implicit facade when a new unnamed gap crosses them.
func hasFloatingFields(d abi.TypeDesc) bool {
	if d.Type == abi.F32 || d.Type == abi.F64 {
		return true
	}
	if d.Type == abi.Array {
		return hasFloatingFields(*d.Elem)
	}
	for _, field := range d.Fields {
		if hasFloatingFields(field.Type) {
			return true
		}
	}
	return false
}

// GCC counts unnamed nonzero bitfields as integer values in small SysV
// records. Clang can ignore them even when every named leaf is floating point.
// A matching memory layout alone does not prove a compatible call ABI.
func onlyFloatingFields(d abi.TypeDesc) bool {
	switch d.Type {
	case abi.F32, abi.F64:
		return true
	case abi.Array:
		return d.Elem != nil && onlyFloatingFields(*d.Elem)
	case abi.Struct:
		if len(d.Fields) == 0 {
			return false
		}
		for _, field := range d.Fields {
			if !onlyFloatingFields(field.Type) {
				return false
			}
		}
		return true
	}
	return false
}
