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
		// __tls_get_addr is an ordinary AAPCS64 C call returning an address.
		// X19 retains the index across the allocator; SP stays 16-byte aligned.
		word(0xa9be7bfd) // stp x29, x30, [sp, #-32]!
		word(0x910003fd) // mov x29, sp
		word(0xf9000bf3) // str x19, [sp, #16]
		word(0xaa0003f3) // mov x19, x0
		word(0xf9400000) // ldr x0, [x0] (private module descriptor)
		for half := uint32(0); half < 4; half++ {
			op := uint32(0xd2800000)
			if half != 0 {
				op = 0xf2800000
			}
			word(op | half<<21 | uint32(uint64(helper)>>(half*16)&0xffff)<<5 | 16)
		}
		word(0xd63f0200) // blr x16
		word(0xf9400670) // ldr x16, [x19, #8] (module byte offset)
		word(0x8b100000) // add x0, x0, x16
		word(0xf9400bf3) // ldr x19, [sp, #16]
		word(0xa8c27bfd) // ldp x29, x30, [sp], #32
		word(0xd65f03c0) // ret
	default:
		return nil, fmt.Errorf("native ELF TLS is unsupported on %s", arch)
	}
	return code, nil
}
