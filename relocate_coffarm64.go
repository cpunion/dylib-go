package dylib

import "fmt"

func (im *image) relocCOFFARM64(r relocation, b []byte, s, p uintptr) error {
	if r.typ == 14 { // ADDR64
		le.PutUint64(b, uint64(s)+le.Uint64(b))
		return nil
	}
	ins := le.Uint32(b)
	switch r.typ {
	case 1: // ADDR32
		return unsigned32(b, int64(s)+int64(ins))
	case 2: // ADDR32NB
		return unsigned32(b, int64(s)-int64(im.base)+int64(ins))
	case 3: // BRANCH26
		return im.armBranch(b, uintptr(int64(s)+signExtend(ins&0x3ffffff, 26)*4), p)
	case 4, 5: // PAGEBASE_REL21 / REL21
		add := signExtend(((ins>>29)&3)|((ins>>5)&0x7ffff)<<2, 21)
		target := uintptr(int64(s) + add)
		if r.typ == 4 {
			// COFF stores this existing immediate as a byte addend, unlike
			// Mach-O's encoded page displacement. Match the COFF convention.
			return armPage(b, target, p)
		}
		if ins&0x9f000000 != 0x10000000 {
			return fmt.Errorf("REL21 does not reference ADR")
		}
		delta := int64(target) - int64(p)
		if delta < -(1<<20) || delta >= (1<<20) {
			return fmt.Errorf("ADR displacement overflow")
		}
		v := uint32(delta) & 0x1fffff
		le.PutUint32(b, (ins&^uint32((3<<29)|(0x7ffff<<5)))|((v&3)<<29)|((v>>2)<<5))
		return nil
	case 6, 7: // PAGEOFFSET_12A / PAGEOFFSET_12L
		scale, err := armOffsetScale(ins)
		if err != nil {
			return err
		}
		if r.typ == 6 && ins&0x5f000000 != 0x11000000 || r.typ == 7 && ins&0x3b000000 != 0x39000000 {
			return fmt.Errorf("COFF page offset relocation has the wrong opcode")
		}
		return armPageOff(b, s+uintptr(((ins>>10)&0xfff)<<scale), scale)
	case 15, 16: // BRANCH19 / BRANCH14
		bits := uint(19)
		if r.typ == 16 {
			bits = 14
			if ins&0x7e000000 != 0x36000000 {
				return fmt.Errorf("BRANCH14 requires TBZ/TBNZ")
			}
		} else if ins&0xff000010 != 0x54000000 && ins&0x7e000000 != 0x34000000 {
			return fmt.Errorf("BRANCH19 requires B.cond/CBZ/CBNZ")
		}
		mask := uint32((1<<bits)-1) << 5
		delta := int64(s) - int64(p) + signExtend((ins&mask)>>5, bits)*4
		if delta%4 != 0 || delta < -(1<<(bits+1)) || delta >= (1<<(bits+1)) {
			return fmt.Errorf("conditional branch displacement overflow or misalignment")
		}
		le.PutUint32(b, (ins&^mask)|((uint32(delta>>2)<<5)&mask))
		return nil
	case 17: // REL32
		return signed32(b, int64(s)-int64(p)-4+int64(int32(ins)))
	default:
		return fmt.Errorf("unsupported ARM64 COFF relocation %d", r.typ)
	}
}
