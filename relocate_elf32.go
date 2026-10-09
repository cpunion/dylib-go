package dylib

import (
	"fmt"
	"math"
)

// GNU uses a baseless memory displacement as an absolute GOT address, but
// keeps a baseless LEA's displacement relative to the table. Inspect original
// bytes so earlier relocation writes cannot change this instruction context.
func elf386AbsoluteGOT(o *object, r relocation) bool {
	if r.section <= 0 || r.section >= len(o.sections) || o.sections[r.section] == nil {
		return false
	}
	b := o.sections[r.section].data
	return r.offset > 1 && r.offset <= uint64(len(b)) && b[r.offset-1]&0xc7 == 5 && b[r.offset-2] != 0x8d
}

func (im *image) relocELF386(o *object, r relocation, b []byte, s, p uintptr) error {
	add := uint32(r.addend)
	if r.implicit {
		add += le.Uint32(b)
	}
	got := im.gotBase()
	var value uint32
	switch r.typ {
	case 1: // R_386_32
		if uint64(s) > math.MaxUint32 {
			return fmt.Errorf("32-bit ELF address overflow")
		}
		value = uint32(s) + add
	case 2, 4: // R_386_PC32 / PLT32
		value = uint32(s) + add - uint32(p)
	case 3, 43: // R_386_GOT32 / GOT32X; no instruction relaxation.
		slot, err := im.gotSlot(s)
		if err != nil {
			return err
		}
		value = uint32(slot) + add - uint32(got)
		if elf386AbsoluteGOT(o, r) {
			value += uint32(got)
		}
	case 9: // R_386_GOTOFF
		value = uint32(s) + add - uint32(got)
	case 10: // R_386_GOTPC
		value = uint32(got) + add - uint32(p)
	default:
		return fmt.Errorf("unsupported i386 ELF relocation %d", r.typ)
	}
	// i386 PC-relative and GOT offsets use modulo-2^32 address arithmetic.
	le.PutUint32(b, value)
	return nil
}
