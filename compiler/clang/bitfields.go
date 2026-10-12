package clang

import (
	"fmt"

	"github.com/cpunion/dylib-go/abi"
)

// Bitfield describes a named integral/Boolean C field, not a Go memory view.
// Mask starts at Layout.Offsets[Position] and contains exactly Width set bits.
// Compiled C adapters restore the declared width and validate this byte mask.
type Bitfield struct {
	Position, Width int
	Mask            []byte
}

// BitfieldPadding describes an unnamed C bitfield before a logical field.
// Before may equal the field count for trailing padding. Width zero requests
// the C compiler's allocation-unit boundary; it is not a Go value field.
type BitfieldPadding struct {
	Before int
	Type   abi.Type
	Width  int
	// Offset is the compiler-confirmed byte boundary for a zero-width field.
	Offset uint64
}

func (r Record) hasBitfields() bool {
	return len(r.Bitfields) != 0 || len(r.Padding) != 0
}

func (r Record) requiresCAdapter() bool {
	return r.Packed || r.hasBitfields() || r.Aligned != 0 || len(r.FieldAlignments) != 0
}

func cloneBitfields(fields []Bitfield) []Bitfield {
	result := append([]Bitfield(nil), fields...)
	for i := range result {
		result[i].Mask = append([]byte(nil), fields[i].Mask...)
	}
	return result
}

func (r Record) bitfieldAt(position int) *Bitfield {
	for i := range r.Bitfields {
		if r.Bitfields[i].Position == position {
			return &r.Bitfields[i]
		}
	}
	return nil
}

func bitfieldTypeBits(kind abi.Type) int {
	switch kind {
	case abi.Bool:
		return 1
	case abi.I8, abi.U8:
		return 8
	case abi.I16, abi.U16:
		return 16
	case abi.I32, abi.U32:
		return 32
	case abi.I64, abi.U64:
		return 64
	default:
		return 0
	}
}

func bitfieldValue(kind abi.Type, width int) string {
	switch kind {
	case abi.I8, abi.I16, abi.I32, abi.I64:
		return "-1"
	default:
		return fmt.Sprintf("%dULL", ^uint64(0)>>(64-width))
	}
}

func validateBitfields(r Record, complete bool) error {
	if len(r.Padding)+len(r.Description.Fields) > 32 {
		return fmt.Errorf("clang: at most 32 named/padding fields in %s", r.Name)
	}
	before := 0
	for _, f := range r.Padding {
		bits := bitfieldTypeBits(f.Type)
		if f.Before < before || f.Before > len(r.Description.Fields) || bits == 0 || f.Width < 0 || f.Width > bits {
			return fmt.Errorf("clang: invalid unnamed bitfield in %s", r.Name)
		}
		if f.Width != 0 && f.Offset != 0 || complete && f.Offset > r.Layout.Size {
			return fmt.Errorf("clang: invalid unnamed bitfield boundary in %s", r.Name)
		}
		before = f.Before
	}
	seen := make(map[int]bool)
	for index, f := range r.Bitfields {
		if f.Position < 0 || f.Position >= len(r.Description.Fields) || seen[f.Position] {
			return fmt.Errorf("clang: invalid or duplicate bitfield position in %s", r.Name)
		}
		seen[f.Position] = true
		d := r.Description.Fields[f.Position].Type
		if f.Width < 1 || f.Width > bitfieldTypeBits(d.Type) || d.Elem != nil || len(f.Mask) > 9 {
			return fmt.Errorf("clang: invalid bitfield width/type in %s", r.Name)
		}
		if len(f.Mask) == 0 {
			if complete {
				return fmt.Errorf("clang: missing bitfield mask in %s", r.Name)
			}
			continue
		}
		count, started, ended := 0, false, false
		for _, b := range f.Mask {
			for bit := uint(0); bit < 8; bit++ {
				if b&(1<<bit) != 0 {
					if ended {
						return fmt.Errorf("clang: noncontiguous bitfield mask in %s", r.Name)
					}
					count++
					started = true
				} else if started {
					ended = true
				}
			}
		}
		if f.Mask[0] == 0 || f.Mask[len(f.Mask)-1] == 0 || count != f.Width {
			return fmt.Errorf("clang: invalid bitfield mask in %s", r.Name)
		}
		if complete {
			if len(r.Layout.Offsets) != len(r.Description.Fields) {
				return fmt.Errorf("clang: missing bitfield offsets in %s", r.Name)
			}
			offset := r.Layout.Offsets[f.Position]
			if offset >= r.Layout.Size || uint64(len(f.Mask)) > r.Layout.Size-offset {
				return fmt.Errorf("clang: bitfield mask outside %s", r.Name)
			}
			for _, previous := range r.Bitfields[:index] {
				base := r.Layout.Offsets[previous.Position]
				for i, b := range f.Mask {
					pos := offset + uint64(i)
					if pos >= base && pos-base < uint64(len(previous.Mask)) && b&previous.Mask[pos-base] != 0 {
						return fmt.Errorf("clang: overlapping bitfield masks in %s", r.Name)
					}
				}
			}
		}
	}
	return nil
}
