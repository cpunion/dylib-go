package dylib

import "fmt"

func armGOTOffset15(b []byte, slot, base, place uintptr) error {
	ins := le.Uint32(b)
	scale, err := armOffsetScale(ins)
	if err != nil || ins&0x3b000000 != 0x39000000 || scale != 3 || place&3 != 0 || ins&(1<<26) == 0 && ins&(1<<23) != 0 {
		return fmt.Errorf("GOT offset requires an aligned 64-bit unsigned-offset load/store")
	}
	offset := uint64(slot) - uint64(base)
	if offset >= 1<<15 || offset&7 != 0 {
		return fmt.Errorf("GOT offset overflow or misalignment")
	}
	le.PutUint32(b, (ins&^uint32(0xfff<<10))|uint32(offset>>3)<<10)
	return nil
}
