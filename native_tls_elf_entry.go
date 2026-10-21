package dylib

import "fmt"

func elfTLSEntry(arch string, helper uintptr) ([]byte, error) {
	if helper == 0 {
		return nil, fmt.Errorf("native ELF TLS requires the native TLS allocator")
	}
	var code []byte
	word := func(ins uint32) {
		var b [4]byte
		le.PutUint32(b[:], ins)
		code = append(code, b[:]...)
	}
	switch arch {
	case "amd64":
		// __tls_get_addr follows the ordinary SysV C ABI. Saving its index
		// also aligns the stack for the native allocator's C call.
		code = append(code, 0x57, 0x48, 0x8b, 0x3f, 0x48, 0xb8)
		var address [8]byte
		le.PutUint64(address[:], uint64(helper))
		code = append(code, address[:]...)
		code = append(code, 0xff, 0xd0, 0x5a, 0x48, 0x03, 0x42, 8, 0xc3)
	case "386":
		if uint64(helper) > 0xffffffff {
			return nil, fmt.Errorf("native i386 TLS allocator exceeds the address space")
		}
		// GNU ___tls_get_addr uses regparm(1), while the allocator is cdecl.
		// Preserve EBP and the incoming stack, then supply a 16-byte aligned
		// C argument area and retain the original index beside its argument.
		code = append(code, 0x55, 0x89, 0xe5, 0x83, 0xe4, 0xf0, 0x83, 0xec, 16)
		code = append(code, 0x89, 0x44, 0x24, 4, 0x8b, 0x00, 0x89, 0x04, 0x24, 0xb8)
		word(uint32(helper))
		code = append(code, 0xff, 0xd0, 0x8b, 0x54, 0x24, 4, 0x03, 0x42, 4, 0x89, 0xec, 0x5d, 0xc3)
	case "arm64":
		// The ELF descriptor returns an offset from TPIDR_EL0, preserving
		// X1-X29 and every bit of Q0-Q31. Ordinary C preserves X19-X29.
		// Unlike Darwin TLV, X16-X18 must also survive the descriptor call.
		word(0xa9bf7bfd) // stp x29, x30, [sp, #-16]!
		word(0x910003fd) // mov x29, sp
		word(0xd10a83ff) // sub sp, sp, #672
		pair := func(op uint32, reg, off, scale int) {
			word(op | uint32(off/scale)<<15 | uint32(reg+1)<<10 | 31<<5 | uint32(reg))
		}
		for reg := 1; reg < 19; reg += 2 {
			pair(0xa9000000, reg, (reg-1)*8, 8)
		}
		word(0xf9004be0) // str x0, [sp, #144] (original descriptor)
		for reg := 0; reg < 32; reg += 2 {
			pair(0xad000000, reg, 160+reg*16, 16)
		}
		word(0xf9400400) // ldr x0, [x0, #8] (descriptor argument)
		word(0xf9400000) // ldr x0, [x0] (private module descriptor)
		for half := uint32(0); half < 4; half++ {
			op := uint32(0xd2800000)
			if half != 0 {
				op = 0xf2800000
			}
			word(op | half<<21 | uint32(uint64(helper)>>(half*16)&0xffff)<<5 | 16)
		}
		word(0xd63f0200) // blr x16
		word(0xf9404bf0) // ldr x16, [sp, #144]
		word(0xf9400610) // ldr x16, [x16, #8] (descriptor argument)
		word(0xf9400610) // ldr x16, [x16, #8] (module byte offset)
		word(0x8b100000) // add x0, x0, x16
		word(0xd53bd050) // mrs x16, TPIDR_EL0
		word(0xcb100000) // sub x0, x0, x16
		for reg := 0; reg < 32; reg += 2 {
			pair(0xad400000, reg, 160+reg*16, 16)
		}
		for reg := 1; reg < 19; reg += 2 {
			pair(0xa9400000, reg, (reg-1)*8, 8)
		}
		word(0x910a83ff) // add sp, sp, #672
		word(0xa8c17bfd) // ldp x29, x30, [sp], #16
		word(0xd65f03c0) // ret
	default:
		return nil, fmt.Errorf("native ELF TLS is unsupported on %s", arch)
	}
	return code, nil
}
