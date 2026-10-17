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
	// TypeAlignment preserves an aligned scalar/enum typedef request, separately
	// from member attributes. StorageAlignment is the qualified bridge boundary;
	// zero means a shared/ignored allocation-unit request.
	TypeAlignment, StorageAlignment uint64
}

// BitfieldPadding describes an unnamed C bitfield before a logical field.
// Before may equal the field count for trailing padding. Width zero requests
// the C compiler's allocation-unit boundary; it is not a Go value field.
type BitfieldPadding struct {
	Before int
	Type   abi.Type
	Width  int
	// Packed preserves an explicit packed attribute on this unnamed declaration.
	Packed bool
	// Aligned preserves an explicit alignment request, including omitted GNU
	// arguments. A producer may ignore it within a shared allocation unit.
	Aligned uint64
	// TypeAlignment retains the aligned base typedef, independently of a
	// declaration's member attribute and its observed allocation boundary.
	TypeAlignment uint64
	// Offset is the compiler-confirmed boundary for a zero-width field, or the
	// first byte of a nonzero-width field with an alignment request or byte mask.
	Offset uint64
	// Mask optionally observes the occupied bytes starting at Offset. Small GNU
	// AMD64 records use this to qualify unnamed-field register classes. It is
	// compiler metadata, never a public value field; a nil mask is unobserved.
	Mask []byte
	// StorageAlignment observes an MS allocation-unit start/active zero-width
	// boundary, or a GNU aligned typedef boundary. Shared requests retain zero.
	StorageAlignment uint64
	// Gap observes bytes before a GNU typedef-aligned allocation boundary or
	// an over-cap pragma zero-width boundary, including plain base types.
	// Bridges preserve this gap without
	// introducing an alignment request that GCC would apply differently.
	Gap uint64
}

func (p BitfieldPadding) cAttributes() string {
	return fieldCAttributes(p.Packed, p.Aligned)
}

func (p BitfieldPadding) requestedAlignment() uint64 {
	if p.TypeAlignment > p.Aligned {
		return p.TypeAlignment
	}
	return p.Aligned
}

// Observe plain zero widths in pragma records as well as attributed ones.
// GCC can raise their root alignment differently, even within the packing cap.
func (r Record) gnuPlainZeroBoundary(p BitfieldPadding) bool {
	return r.MaxFieldAlignment != 0 && p.Width == 0 && p.requestedAlignment() == 0
}

func (r Record) hasBitfields() bool {
	return len(r.Bitfields) != 0 || len(r.Padding) != 0
}

func (r Record) requiresCAdapter() bool {
	return r.Packed || r.MaxFieldAlignment != 0 || len(r.PackedFields) != 0 || r.hasBitfields() || r.Aligned != 0 || len(r.FieldAlignments) != 0 || len(r.FieldTypeAlignments) != 0 || len(r.TypedefAlignments) != 0 || r.IndirectArgument || r.IndirectResult || r.ArgumentAlignment != 0
}

func cloneBitfields(fields []Bitfield) []Bitfield {
	result := append([]Bitfield(nil), fields...)
	for i := range result {
		result[i].Mask = append([]byte(nil), fields[i].Mask...)
	}
	return result
}

func clonePadding(fields []BitfieldPadding) []BitfieldPadding {
	result := append([]BitfieldPadding(nil), fields...)
	for i := range result {
		result[i].Mask = append([]byte(nil), fields[i].Mask...)
	}
	return result
}

func validateBitfieldMask(mask []byte, width int) error {
	if len(mask) == 0 || len(mask) > 9 || mask[0] == 0 || mask[len(mask)-1] == 0 {
		return fmt.Errorf("invalid bitfield mask")
	}
	count, started, ended := 0, false, false
	for _, b := range mask {
		for bit := uint(0); bit < 8; bit++ {
			if b&(1<<bit) != 0 {
				if ended {
					return fmt.Errorf("noncontiguous bitfield mask")
				}
				count++
				started = true
			} else if started {
				ended = true
			}
		}
	}
	if count != width {
		return fmt.Errorf("invalid bitfield mask")
	}
	return nil
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
		if f.Before < before || f.Before > len(r.Description.Fields) || bits == 0 || f.Width < 0 || f.Width > bits || !validAlignment(f.Aligned) || !validAlignment(f.TypeAlignment) || !validAlignment(f.StorageAlignment) || f.StorageAlignment != 0 && r.MaxFieldAlignment == 0 && f.TypeAlignment == 0 {
			return fmt.Errorf("clang: invalid unnamed bitfield in %s", r.Name)
		}
		if f.Width != 0 && f.requestedAlignment() == 0 && len(f.Mask) == 0 && f.Offset != 0 || complete && (f.Offset > r.Layout.Size || f.Width != 0 && (f.requestedAlignment() != 0 || len(f.Mask) != 0) && f.Offset == r.Layout.Size) {
			return fmt.Errorf("clang: invalid unnamed bitfield boundary in %s", r.Name)
		}
		if len(f.Mask) != 0 {
			if err := validateBitfieldMask(f.Mask, f.Width); err != nil {
				return fmt.Errorf("clang: unnamed %w in %s", err, r.Name)
			}
			if complete && (f.Offset >= r.Layout.Size || uint64(len(f.Mask)) > r.Layout.Size-f.Offset) {
				return fmt.Errorf("clang: unnamed bitfield mask outside %s", r.Name)
			}
		}
		if complete && f.StorageAlignment != 0 && (f.StorageAlignment > r.Layout.Alignment && f.TypeAlignment == 0 || (f.Width == 0 || f.requestedAlignment() != 0) && f.Offset%f.StorageAlignment != 0) {
			return fmt.Errorf("clang: incompatible unnamed storage alignment in %s", r.Name)
		}
		gapAllowed := f.TypeAlignment != 0 || r.gnuPlainZeroBoundary(f) || f.Width == 0 && r.MaxFieldAlignment != 0 && f.Aligned > r.MaxFieldAlignment
		if f.Gap > 65536 || f.Gap != 0 && (!gapAllowed || complete && f.Gap > f.Offset) {
			return fmt.Errorf("clang: invalid bitfield boundary gap in %s", r.Name)
		}
		before = f.Before
	}
	seen := make(map[int]bool)
	for index, f := range r.Bitfields {
		if f.Position < 0 || f.Position >= len(r.Description.Fields) || seen[f.Position] {
			return fmt.Errorf("clang: invalid or duplicate bitfield position in %s", r.Name)
		}
		seen[f.Position] = true
		if !validAlignment(f.TypeAlignment) || !validAlignment(f.StorageAlignment) || f.TypeAlignment == 0 && f.StorageAlignment != 0 || complete && f.StorageAlignment > r.Layout.Alignment {
			return fmt.Errorf("clang: invalid bitfield typedef alignment in %s", r.Name)
		}
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
		if err := validateBitfieldMask(f.Mask, f.Width); err != nil {
			return fmt.Errorf("clang: %w in %s", err, r.Name)
		}
		if complete {
			if len(r.Layout.Offsets) != len(r.Description.Fields) {
				return fmt.Errorf("clang: missing bitfield offsets in %s", r.Name)
			}
			offset := r.Layout.Offsets[f.Position]
			if f.StorageAlignment != 0 && (offset%f.StorageAlignment != 0 || f.Mask[0]&1 == 0) {
				return fmt.Errorf("clang: incompatible bitfield typedef boundary in %s", r.Name)
			}
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
