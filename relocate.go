package dylib

import (
	"encoding/binary"
	"fmt"
	"math"
)

var le = binary.LittleEndian

func signed32(b []byte, v int64) error {
	if v < math.MinInt32 || v > math.MaxInt32 {
		return fmt.Errorf("signed 32-bit relocation overflow: %d", v)
	}
	le.PutUint32(b, uint32(v))
	return nil
}
func unsigned32(b []byte, v int64) error {
	if v < 0 || v > math.MaxUint32 {
		return fmt.Errorf("unsigned 32-bit relocation overflow: %d", v)
	}
	le.PutUint32(b, uint32(v))
	return nil
}
func signExtend(v uint32, bits uint) int64 { return int64(int32(v<<(32-bits)) >> (32 - bits)) }

func (im *image) relocate(o *object, r relocation) error {
	if r.section <= 0 || r.section >= len(o.sections) || o.sections[r.section] == nil {
		return fmt.Errorf("invalid target section")
	}
	sec := o.sections[r.section]
	w := 4
	switch o.info.Format {
	case "Mach-O":
		w = r.width
		if w != 4 && w != 8 {
			return fmt.Errorf("unsupported Mach-O relocation width %d", w)
		}
	case "ELF":
		if r.typ == 0 {
			return nil
		}
		if o.info.Arch == "amd64" && (r.typ == 1 || r.typ == 24) || o.info.Arch == "arm64" && (r.typ == 257 || r.typ == 260) {
			w = 8
		}
	case "COFF":
		if r.typ == 0 {
			return nil
		}
		if r.typ == 1 {
			w = 8
		}
	}
	if r.offset > sec.size || uint64(w) > sec.size-r.offset {
		return fmt.Errorf("relocation writes outside section")
	}
	b := im.mem[sec.offset+r.offset : sec.offset+r.offset+uint64(w)]
	p := im.base + uintptr(sec.offset+r.offset)
	s, e := im.symbol(o, r.symbol, r.local)
	if e != nil {
		return e
	}
	if r.pair >= 0 {
		if r.local || r.pcrel {
			return fmt.Errorf("invalid subtractor flags")
		}
		end, e := im.symbol(o, r.pair, false)
		if e != nil {
			return e
		}
		if w == 8 {
			le.PutUint64(b, uint64(int64(end)-int64(s)+int64(le.Uint64(b))+r.addend))
			return nil
		}
		return signed32(b, int64(end)-int64(s)+int64(int32(le.Uint32(b)))+r.addend)
	}
	switch o.info.Format {
	case "ELF":
		return im.relocELF(o, r, b, s, p)
	case "Mach-O":
		return im.relocMachO(o, r, b, s, p)
	case "COFF":
		return im.relocCOFF(o, r, b, s, p)
	}
	return fmt.Errorf("no relocation backend")
}

func (im *image) relocELF(o *object, r relocation, b []byte, s, p uintptr) error {
	v := int64(s) + r.addend
	if o.info.Arch == "amd64" {
		switch r.typ {
		case 1:
			le.PutUint64(b, uint64(v))
			return nil
		case 2:
			return signed32(b, v-int64(p))
		case 4:
			d := v - int64(p)
			if d < math.MinInt32 || d > math.MaxInt32 {
				stub, e := im.stub(s, "amd64")
				if e != nil {
					return e
				}
				d = int64(stub) + r.addend - int64(p)
			}
			return signed32(b, d)
		case 9, 41, 42:
			g, e := im.gotSlot(s)
			if e != nil {
				return e
			}
			return signed32(b, int64(g)+r.addend-int64(p))
		case 10:
			return unsigned32(b, v)
		case 11:
			return signed32(b, v)
		case 24:
			le.PutUint64(b, uint64(v-int64(p)))
			return nil
		}
	} else if o.info.Arch == "arm64" {
		switch r.typ {
		case 257:
			le.PutUint64(b, uint64(v))
			return nil
		case 258:
			return unsigned32(b, v)
		case 260:
			le.PutUint64(b, uint64(v-int64(p)))
			return nil
		case 261:
			return signed32(b, v-int64(p))
		case 275, 276:
			return armPage(b, uintptr(v), p)
		case 277:
			return armPageOff(b, uintptr(v), 0)
		case 278:
			return armPageOff(b, uintptr(v), 0)
		case 284:
			return armPageOff(b, uintptr(v), 1)
		case 285:
			return armPageOff(b, uintptr(v), 2)
		case 286:
			return armPageOff(b, uintptr(v), 3)
		case 299:
			return armPageOff(b, uintptr(v), 4)
		case 282, 283:
			return im.armBranch(b, uintptr(v), p)
		case 311:
			g, e := im.gotSlot(s)
			if e != nil {
				return e
			}
			return armPage(b, uintptr(int64(g)+r.addend), p)
		case 312:
			g, e := im.gotSlot(s)
			if e != nil {
				return e
			}
			return armPageOff(b, uintptr(int64(g)+r.addend), 3)
		}
	}
	return fmt.Errorf("unsupported %s ELF relocation %d", o.info.Arch, r.typ)
}

func (im *image) relocMachO(o *object, r relocation, b []byte, s, p uintptr) error {
	if r.typ == 0 {
		if r.pcrel {
			return fmt.Errorf("PC-relative unsigned relocation")
		}
		v := uint64(s) + uint64(r.addend)
		if len(b) == 8 {
			v += le.Uint64(b)
		} else {
			v += uint64(le.Uint32(b))
		}
		if r.local {
			v -= o.sections[r.symbol].original
		}
		if len(b) == 8 {
			le.PutUint64(b, v)
			return nil
		}
		return unsigned32(b, int64(v))
	}
	if len(b) != 4 {
		return fmt.Errorf("instruction/PC-relative relocation requires 4 bytes")
	}
	if o.info.Arch == "amd64" {
		if !r.pcrel {
			return fmt.Errorf("x86-64 relocation requires PC-relative flag")
		}
		add := int64(int32(le.Uint32(b))) + r.addend
		bias := int64(4)
		if r.local {
			add += int64(o.sections[r.section].original+r.offset+4) - int64(o.sections[r.symbol].original)
		}
		switch r.typ {
		case 1:
		case 2:
			d := int64(s) + add - int64(p) - bias
			if d < math.MinInt32 || d > math.MaxInt32 {
				var e error
				s, e = im.stub(s, "amd64")
				if e != nil {
					return e
				}
			}
		case 3, 4:
			var e error
			s, e = im.gotSlot(s)
			if e != nil {
				return e
			}
		case 6, 7, 8:
			// SIGNED_1/2/4 encode the trailing immediate-byte correction
			// in the stored addend already. Subtracting it again is wrong.
		default:
			return fmt.Errorf("unsupported x86-64 Mach-O relocation %d", r.typ)
		}
		return signed32(b, int64(s)+add-int64(p)-bias)
	}
	if o.info.Arch == "arm64" {
		if r.local {
			return fmt.Errorf("ARM64 instruction relocations require external symbol indices")
		}
		ins := le.Uint32(b)
		switch r.typ {
		case 2:
			if !r.pcrel {
				return fmt.Errorf("BRANCH26 requires PC-relative flag")
			}
			return im.armBranch(b, uintptr(int64(s)+r.addend+signExtend(ins&0x3ffffff, 26)*4), p)
		case 3, 5:
			if !r.pcrel {
				return fmt.Errorf("PAGE21 requires PC-relative flag")
			}
			if r.typ == 5 {
				var e error
				s, e = im.gotSlot(s)
				if e != nil {
					return e
				}
			}
			add := signExtend(((ins>>29)&3)|((ins>>5)&0x7ffff)<<2, 21) << 12
			return armPage(b, uintptr(int64(s)+r.addend+add), p)
		case 4, 6:
			if r.pcrel {
				return fmt.Errorf("PAGEOFF12 must not be PC-relative")
			}
			if r.typ == 6 {
				var e error
				s, e = im.gotSlot(s)
				if e != nil {
					return e
				}
			}
			shift, e := armOffsetScale(ins)
			if e != nil {
				return e
			}
			add := int64((ins>>10)&0xfff) << shift
			return armPageOff(b, uintptr(int64(s)+r.addend+add), shift)
		case 7:
			if !r.pcrel {
				return fmt.Errorf("POINTER_TO_GOT requires PC-relative flag")
			}
			g, e := im.gotSlot(s)
			if e != nil {
				return e
			}
			return signed32(b, int64(g)+r.addend+int64(int32(ins))-int64(p))
		}
	}
	return fmt.Errorf("unsupported %s Mach-O relocation %d", o.info.Arch, r.typ)
}

func (im *image) relocCOFF(o *object, r relocation, b []byte, s, p uintptr) error {
	if o.info.Arch != "amd64" {
		return fmt.Errorf("only AMD64 COFF relocation implemented")
	}
	switch r.typ {
	case 1:
		le.PutUint64(b, uint64(s)+le.Uint64(b))
		return nil
	case 2:
		return unsigned32(b, int64(s)+int64(le.Uint32(b)))
	case 3:
		return unsigned32(b, int64(s)-int64(im.base)+int64(le.Uint32(b)))
	case 4, 5, 6, 7, 8, 9:
		add := int64(int32(le.Uint32(b)))
		d := int64(s) + add - int64(p) - 4 - int64(r.typ-4)
		if d < math.MinInt32 || d > math.MaxInt32 {
			sec := o.sections[r.section]
			off := sec.offset + r.offset
			// REL32 also relocates data; only a direct call/jmp can use a stub.
			if r.offset == 0 || (im.mem[off-1] != 0xe8 && im.mem[off-1] != 0xe9) {
				return fmt.Errorf("COFF data reference out of range; compile position-independent code or use __declspec(dllimport)")
			}
			stub, e := im.stub(s, "amd64")
			if e != nil {
				return e
			}
			d = int64(stub) + add - int64(p) - 4 - int64(r.typ-4)
		}
		return signed32(b, d)
	default:
		return fmt.Errorf("unsupported AMD64 COFF relocation %d", r.typ)
	}
}

func armPage(b []byte, target, p uintptr) error {
	ins := le.Uint32(b)
	if ins&0x9f000000 != 0x90000000 {
		return fmt.Errorf("PAGE21 relocation does not reference ADRP")
	}
	d := (int64(target&^0xfff) - int64(p&^0xfff)) >> 12
	if d < -(1<<20) || d >= (1<<20) {
		return fmt.Errorf("ADRP page displacement overflow")
	}
	v := uint32(d) & 0x1fffff
	ins = (ins &^ uint32((3<<29)|(0x7ffff<<5))) | ((v & 3) << 29) | ((v >> 2) << 5)
	le.PutUint32(b, ins)
	return nil
}
func armOffsetScale(ins uint32) (uint, error) {
	if ins&0x7f000000 == 0x11000000 {
		if ins&(1<<22) != 0 {
			return 0, fmt.Errorf("shifted ADD not supported for PAGEOFF12")
		}
		return 0, nil
	}
	if ins&0x3b000000 == 0x39000000 {
		scale := uint(ins >> 30)
		if ins&(1<<26) != 0 && ins&(1<<23) != 0 {
			scale = 4
		}
		return scale, nil
	}
	return 0, fmt.Errorf("PAGEOFF12 requires ADD or unsigned-offset load/store")
}
func armPageOff(b []byte, target uintptr, scale uint) error {
	ins := le.Uint32(b)
	got, e := armOffsetScale(ins)
	if e != nil {
		return e
	}
	if got != scale {
		return fmt.Errorf("load/store scale mismatch")
	}
	off := uint32(target & 0xfff)
	if off&((1<<scale)-1) != 0 {
		return fmt.Errorf("unaligned page offset")
	}
	le.PutUint32(b, (ins&^uint32(0xfff<<10))|((off>>scale)<<10))
	return nil
}
func (im *image) armBranch(b []byte, target, p uintptr) error {
	ins := le.Uint32(b)
	if ins&0x7c000000 != 0x14000000 {
		return fmt.Errorf("BRANCH26 does not reference B/BL")
	}
	d := int64(target) - int64(p)
	if d%4 != 0 {
		return fmt.Errorf("unaligned branch target")
	}
	if d < -(1<<27) || d >= (1<<27) {
		stub, e := im.stub(target, "arm64")
		if e != nil {
			return e
		}
		d = int64(stub) - int64(p)
	}
	if d%4 != 0 || d < -(1<<27) || d >= (1<<27) {
		return fmt.Errorf("branch stub out of range")
	}
	le.PutUint32(b, (ins&0xfc000000)|(uint32(d>>2)&0x3ffffff))
	return nil
}
