package dylib

import "fmt"

// AArch64 ABS data permits signed or unsigned narrow values; PREL is signed.
// Data places may be byte-aligned, unlike instructions.
func armDataRelocation(b []byte, value int64, signed bool) error {
	bits := uint(len(b) * 8)
	upper := int64(1)<<bits - 1
	if signed {
		upper = int64(1)<<(bits-1) - 1
	}
	if value < -(int64(1)<<(bits-1)) || value > upper {
		return fmt.Errorf("AArch64 %d-bit data relocation overflow: %d", bits, value)
	}
	if len(b) == 2 {
		le.PutUint16(b, uint16(value))
	} else {
		le.PutUint32(b, uint32(value))
	}
	return nil
}

func armELFMOVW(b []byte, typ uint32, value uint64, place uintptr) error {
	var group uint
	unsigned, checked := false, true
	switch {
	case typ >= 263 && typ <= 269: // MOVW_UABS_G0..G3, with _NC lower groups.
		group, unsigned = uint((typ-263)/2), true
		checked = typ&1 != 0
	case typ >= 270 && typ <= 272: // MOVW_SABS_G0..G2.
		group = uint(typ - 270)
	case typ >= 287 && typ <= 293: // MOVW_PREL_G0..G3.
		group = uint((typ - 287) / 2)
		checked = typ&1 != 0
		value -= uint64(place)
	case typ >= 300 && typ <= 306: // MOVW_GOTOFF_G0..G3; value is slot-GOT.
		group = uint((typ - 300) / 2)
		checked = typ&1 == 0
	default:
		return fmt.Errorf("unsupported AArch64 MOVW relocation %d", typ)
	}
	ins := le.Uint32(b)
	opcode := (ins >> 29) & 3
	if place&3 != 0 || ins&0x1f800000 != 0x12800000 || ins>>31 == 0 && ins&(2<<21) != 0 {
		return fmt.Errorf("MOVW requires an aligned, valid move-wide instruction")
	}
	if unsigned && opcode != 2 && opcode != 3 || !unsigned && checked && opcode != 0 && opcode != 2 || !unsigned && !checked && opcode != 3 {
		return fmt.Errorf("MOVW relocation %d has an incompatible opcode", typ)
	}
	shift := group * 16
	if checked && group != 3 {
		bits := shift + 16
		if unsigned && value>>bits != 0 || !unsigned && (int64(value) < -(int64(1)<<bits) || int64(value) >= int64(1)<<bits) {
			return fmt.Errorf("MOVW relocation %d overflow", typ)
		}
	}
	field := uint32(value>>shift) & 0xffff
	if !unsigned && checked {
		if int64(value) < 0 {
			ins &^= 1 << 30 // MOVN uses the complemented immediate.
			field ^= 0xffff
		} else {
			ins |= 1 << 30 // MOVZ.
		}
	}
	// The relocation selects source bits. Retain the instruction's destination
	// shift, width and register, even when its shift differs from that group.
	le.PutUint32(b, (ins&^uint32(0xffff<<5))|field<<5)
	return nil
}
