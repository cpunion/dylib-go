package dylib

import "fmt"

// Callers supply the full target after applying their format's addend. ELF
// RELA overwrites the immediate; COFF first decodes its implicit addend.
func armADR(b []byte, target, p uintptr) error {
	ins := le.Uint32(b)
	if ins&0x9f000000 != 0x10000000 || p&3 != 0 {
		return fmt.Errorf("ADR relocation requires an aligned ADR instruction")
	}
	delta := int64(target) - int64(p)
	if delta < -(1<<20) || delta >= (1<<20) {
		return fmt.Errorf("ADR displacement overflow")
	}
	v := uint32(delta) & 0x1fffff
	le.PutUint32(b, (ins&^uint32((3<<29)|(0x7ffff<<5)))|((v&3)<<29)|((v>>2)<<5))
	return nil
}

func armScaledPCRelative(b []byte, target, p uintptr, bits uint) error {
	delta := int64(target) - int64(p)
	if p&3 != 0 || delta%4 != 0 || delta < -(1<<(bits+1)) || delta >= (1<<(bits+1)) {
		return fmt.Errorf("ARM PC-relative displacement overflow or misalignment")
	}
	mask := uint32((1<<bits)-1) << 5
	le.PutUint32(b, (le.Uint32(b)&^mask)|((uint32(delta>>2)<<5)&mask))
	return nil
}

func armLiteralLoad(b []byte, target, p uintptr) error {
	// Integer/SIMD LDR, LDRSW and PRFM literals share this encoding class.
	ins := le.Uint32(b)
	if ins&0x3b000000 != 0x18000000 || ins&(1<<26) != 0 && ins>>30 == 3 {
		return fmt.Errorf("literal relocation requires LDR, LDRSW or PRFM")
	}
	return armScaledPCRelative(b, target, p, 19)
}

func armConditionalBranch(b []byte, target, p uintptr, bits uint) error {
	ins := le.Uint32(b)
	if bits == 14 {
		if ins&0x7e000000 != 0x36000000 {
			return fmt.Errorf("BRANCH14 requires TBZ/TBNZ")
		}
	} else if bits != 19 || ins&0xff000010 != 0x54000000 && ins&0x7e000000 != 0x34000000 {
		return fmt.Errorf("BRANCH19 requires B.cond/CBZ/CBNZ")
	}
	return armScaledPCRelative(b, target, p, bits)
}

func (im *image) elfARM64GOT(r relocation, target uintptr) (uintptr, error) {
	// AAELF64 requires a zero addend for every relocation using GDAT(S).
	if r.addend != 0 {
		return 0, fmt.Errorf("AArch64 ELF GOT relocation %d requires a zero addend", r.typ)
	}
	return im.gotSlot(target)
}
