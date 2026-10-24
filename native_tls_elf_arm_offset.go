package dylib

import "fmt"

// Canonical 64-bit TLS move-wide operands. Checked forms select MOVN/MOVZ;
// unchecked forms retain MOVK. The existing ELF encoder owns sign and range.
func armELFTLSMOVW(b []byte, group uint, checked bool, value uint64, place uintptr) error {
	ins := le.Uint32(b)
	want, mask := uint32(0xf2800000)|uint32(group)<<21, uint32(31)
	typ := uint32(264 + 2*group) // MOVW_UABS_Gn_NC supplies the selected bits.
	if checked {
		want, mask = 0x92800000|uint32(group)<<21, 1<<30|31
		typ = 270 + uint32(group) // MOVW_SABS_Gn selects sign and checks range.
	}
	if group > 2 || ins&^mask != want {
		return fmt.Errorf("TLS MOVW requires a zero-immediate 64-bit move with matching opcode and shift")
	}
	return armELFMOVW(b, typ, value, place)
}

// DTPREL uses the packed image template offset, independent of the address of
// the private module index and the platform's thread pointer or static TLS.
func armELFTLSOffset(b []byte, typ uint32, value, remaining uint64, place uintptr) error {
	ins := le.Uint32(b)
	if typ >= 523 && typ <= 527 {
		group := uint(2)
		if typ != 523 {
			group = uint((527 - typ) / 2)
		}
		return armELFTLSMOVW(b, group, typ == 523 || typ == 524 || typ == 526, value, place)
	}
	if typ >= 528 && typ <= 530 {
		want, shift, limit := uint32(0x91000000), uint(0), uint64(1)<<12
		if typ == 528 {
			want, shift, limit = 0x91400000, 12, 1<<24
		}
		if ins&0xfffffc00 != want || typ != 530 && value >= limit {
			return fmt.Errorf("TLS DTPREL ADD requires a zero-immediate 64-bit ADD within its field range")
		}
		le.PutUint32(b, ins|uint32(value>>shift&4095)<<10)
		return nil
	}
	var scale uint
	checked := typ&1 != 0
	if typ >= 531 && typ <= 538 {
		scale = uint((typ - 531) / 2)
	} else if typ == 572 || typ == 573 {
		scale, checked = 4, typ == 572
	} else {
		return fmt.Errorf("unsupported AArch64 DTPREL relocation %d", typ)
	}
	// Unsigned-offset integer or SIMD LDR/STR, with an exact datum width.
	// Sign-extending loads, prefetch and alternate addressing forms are unqualified.
	want, mask := uint32(0x39000000)|uint32(scale)<<30, uint32(1<<22|1<<26|1023)
	if scale == 4 {
		want, mask = 0x3d800000, 1<<22|1023
	}
	if ins&^mask != want || ins&(4095<<10) != 0 {
		return fmt.Errorf("TLS DTPREL load/store requires a zero-offset unsigned LDR/STR of the declared width")
	}
	if value&((1<<scale)-1) != 0 || remaining < 1<<scale || checked && value >= 4096 {
		return fmt.Errorf("TLS DTPREL load/store offset exceeds its range, alignment or template extent")
	}
	return armPageOff(b, uintptr(value), scale)
}
