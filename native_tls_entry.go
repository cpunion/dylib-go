package dylib

import "fmt"

// Darwin TLV lookup preserves more registers than an ordinary C call. Keep the
// compiler's TLVP callers intact and adapt to the existing native TLS allocator.
// The descriptor is [entry, private module descriptor, module byte offset].
func machoTLSEntry(arch string, helper uintptr) ([]byte, error) {
	if helper == 0 {
		return nil, fmt.Errorf("native Mach-O TLS requires the native TLS allocator")
	}
	var code []byte
	word := func(instruction uint32) {
		var b [4]byte
		le.PutUint32(b[:], instruction)
		code = append(code, b[:]...)
	}
	switch arch {
	case "amd64":
		// CSR_64_TLS_Darwin: all GPRs except RDI/RAX are preserved.
		// Save the seven extra caller-saved GPRs and the descriptor; reserve
		// another eight bytes to maintain a 16-byte C call boundary.
		code = append(code, 0x51, 0x52, 0x56, 0x41, 0x50, 0x41, 0x51, 0x41, 0x52, 0x41, 0x53, 0x57)
		code = append(code, 0x48, 0x83, 0xec, 8) // sub rsp, 8
		code = append(code, 0x48, 0x8b, 0x7f, 8) // mov rdi, [rdi+8]
		code = append(code, 0x48, 0xb8)          // movabs rax, native allocator
		var address [8]byte
		le.PutUint64(address[:], uint64(helper))
		code = append(code, address[:]...)
		code = append(code, 0xff, 0xd0)                // call rax
		code = append(code, 0x48, 0x8b, 0x7c, 0x24, 8) // mov rdi, [rsp+8]
		code = append(code, 0x48, 0x03, 0x47, 16)      // add rax, [rdi+16]
		code = append(code, 0x48, 0x83, 0xc4, 8)       // add rsp, 8
		code = append(code, 0x5f, 0x41, 0x5b, 0x41, 0x5a, 0x41, 0x59, 0x41, 0x58, 0x5e, 0x5a, 0x59, 0xc3)
	case "arm64":
		// CSR_Darwin_AArch64_TLS preserves X1-X15 and all 128 bits of
		// Q0-Q31. Ordinary C preserves X18-X29; X16/X17 remain scratch.
		word(0xa9bf7bfd) // stp x29, x30, [sp, #-16]!
		word(0x910003fd) // mov x29, sp
		word(0xd10a03ff) // sub sp, sp, #640
		pair := func(opcode uint32, reg, offset, scale int) {
			word(opcode | uint32(offset/scale)<<15 | uint32(reg+1)<<10 | 31<<5 | uint32(reg))
		}
		for reg := 1; reg < 15; reg += 2 {
			pair(0xa9000000, reg, (reg-1)*8, 8) // stp X pairs
		}
		word(0xf9003bef) // str x15, [sp, #112]
		word(0xf9003fe0) // str x0, [sp, #120] (original TLV descriptor)
		for reg := 0; reg < 32; reg += 2 {
			pair(0xad000000, reg, 128+reg*16, 16) // stp Q pairs
		}
		word(0xf9400400) // ldr x0, [x0, #8] (private module descriptor)
		for half := uint32(0); half < 4; half++ {
			opcode := uint32(0xd2800000) // movz x16, imm16
			if half != 0 {
				opcode = 0xf2800000 // movk x16, imm16, lsl #(half*16)
			}
			word(opcode | half<<21 | uint32(uint64(helper)>>(half*16)&0xffff)<<5 | 16)
		}
		word(0xd63f0200) // blr x16
		word(0xf9403ff0) // ldr x16, [sp, #120]
		word(0xf9400a10) // ldr x16, [x16, #16] (module byte offset)
		word(0x8b100000) // add x0, x0, x16
		for reg := 0; reg < 32; reg += 2 {
			pair(0xad400000, reg, 128+reg*16, 16) // ldp Q pairs
		}
		for reg := 1; reg < 15; reg += 2 {
			pair(0xa9400000, reg, (reg-1)*8, 8) // ldp X pairs
		}
		word(0xf9403bef) // ldr x15, [sp, #112]
		word(0x910a03ff) // add sp, sp, #640
		word(0xa8c17bfd) // ldp x29, x30, [sp], #16
		word(0xd65f03c0) // ret
	default:
		return nil, fmt.Errorf("native Mach-O TLS is unsupported on %s", arch)
	}
	return code, nil
}
