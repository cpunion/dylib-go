package dylib

import "fmt"

func elfX86Narrow(o *object, typ uint32) (width int, relative bool) {
	if o.info.Format != "ELF" {
		return 0, false
	}
	if o.info.Arch == "amd64" {
		switch typ {
		case 12, 13: // R_X86_64_16 / PC16
			return 2, typ == 13
		case 14, 15: // R_X86_64_8 / PC8
			return 1, typ == 15
		}
	} else if o.info.Arch == "386" {
		switch typ {
		case 20, 21: // R_386_16 / PC16
			return 2, typ == 21
		case 22, 23: // R_386_8 / PC8
			return 1, typ == 23
		}
	}
	return 0, false
}

// REL stores a signed addend in the actual one/two-byte field. RELA keeps it
// separately. Match LLVM's field checks; i386 PC16 additionally allows the
// signed 17-bit displacement used by wrapping 16-bit instruction pointers.
func relocateELFX86Narrow(o *object, r relocation, b []byte, s, p uintptr, relative bool) error {
	value := int64(s) + r.addend
	if r.implicit {
		if len(b) == 1 {
			value += int64(int8(b[0]))
		} else {
			value += int64(int16(le.Uint16(b)))
		}
	}
	if relative {
		value -= int64(p)
	}
	if o.info.Arch == "386" {
		value = int64(int32(value)) // Native address arithmetic wraps at 32 bits.
	}
	bits := uint(len(b) * 8)
	minimum, maximum := -int64(1)<<(bits-1), int64(1)<<bits-1
	if relative {
		maximum = int64(1)<<(bits-1) - 1
		if o.info.Arch == "386" && bits == 16 {
			minimum, maximum = -1<<16, 1<<16-1
		}
	}
	if value < minimum || value > maximum {
		return fmt.Errorf("%d-bit x86 ELF relocation overflow: %d", bits, value)
	}
	if len(b) == 1 {
		b[0] = byte(value)
	} else {
		le.PutUint16(b, uint16(value))
	}
	return nil
}
