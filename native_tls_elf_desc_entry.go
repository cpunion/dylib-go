package dylib

import "fmt"

// elfTLSDescEntry adapts the register-preserving x86 TLSDESC ABI to the native
// allocator. It returns the address minus FS:0 (amd64) or GS:0 (i386).
// XSAVE preserves all OS-enabled user state (including x87 and wide vectors);
// CPUs without OSXSAVE use FXSAVE. CPUID supplies the enabled-state area size.
// The independent assembler reference is testdata/elf_tlsdesc_entry.S.
func elfTLSDescEntry(arch string, helper uintptr) ([]byte, error) {
	if helper == 0 {
		return nil, fmt.Errorf("native ELF TLSDESC requires the native TLS allocator")
	}
	if arch == "386" && uint64(helper) > 0xffffffff {
		return nil, fmt.Errorf("native i386 TLSDESC allocator exceeds the address space")
	}
	var code []byte
	switch arch {
	case "amd64":
		code = append(code, 0xf3, 0x0f, 0x1e, 0xfa)                                     // endbr64
		code = append(code, 0x55)                                                       // pushq	%rbp
		code = append(code, 0x48, 0x89, 0xe5)                                           // movq	%rsp, %rbp
		code = append(code, 0x53)                                                       // pushq	%rbx
		code = append(code, 0x51)                                                       // pushq	%rcx
		code = append(code, 0x52)                                                       // pushq	%rdx
		code = append(code, 0x56)                                                       // pushq	%rsi
		code = append(code, 0x57)                                                       // pushq	%rdi
		code = append(code, 0x41, 0x50)                                                 // pushq	%r8
		code = append(code, 0x41, 0x51)                                                 // pushq	%r9
		code = append(code, 0x41, 0x52)                                                 // pushq	%r10
		code = append(code, 0x41, 0x53)                                                 // pushq	%r11
		code = append(code, 0x50)                                                       // pushq	%rax
		code = append(code, 0x6a, 0x00)                                                 // pushq	$0x0
		code = append(code, 0x6a, 0x00)                                                 // pushq	$0x0
		code = append(code, 0x48, 0x83, 0xe4, 0xc0)                                     // andq	$-0x40, %rsp
		code = append(code, 0xb8, 0x01, 0x00, 0x00, 0x00)                               // movl	$0x1, %eax
		code = append(code, 0x0f, 0xa2)                                                 // cpuid
		code = append(code, 0xf7, 0xc1, 0x00, 0x00, 0x00, 0x08)                         // testl	$0x8000000, %ecx
		code = append(code, 0x74, 0x18)                                                 // je	0x45
		code = append(code, 0x31, 0xc9)                                                 // xorl	%ecx, %ecx
		code = append(code, 0x0f, 0x01, 0xd0)                                           // xgetbv
		code = append(code, 0x89, 0x45, 0xa8)                                           // movl	%eax, -0x58(%rbp)
		code = append(code, 0x89, 0x55, 0xa0)                                           // movl	%edx, -0x60(%rbp)
		code = append(code, 0xb8, 0x0d, 0x00, 0x00, 0x00)                               // movl	$0xd, %eax
		code = append(code, 0x31, 0xc9)                                                 // xorl	%ecx, %ecx
		code = append(code, 0x0f, 0xa2)                                                 // cpuid
		code = append(code, 0x89, 0xd8)                                                 // movl	%ebx, %eax
		code = append(code, 0xeb, 0x05)                                                 // jmp	0x4a
		code = append(code, 0xb8, 0x00, 0x02, 0x00, 0x00)                               // movl	$0x200, %eax
		code = append(code, 0x83, 0xc0, 0x3f)                                           // addl	$0x3f, %eax
		code = append(code, 0x83, 0xe0, 0xc0)                                           // andl	$-0x40, %eax
		code = append(code, 0x48, 0x29, 0xc4)                                           // subq	%rax, %rsp
		code = append(code, 0x83, 0x7d, 0xa8, 0x00)                                     // cmpl	$0x0, -0x58(%rbp)
		code = append(code, 0x74, 0x1f)                                                 // je	0x78
		code = append(code, 0x48, 0x8d, 0xbc, 0x24, 0x00, 0x02, 0x00, 0x00)             // leaq	0x200(%rsp), %rdi
		code = append(code, 0x31, 0xc0)                                                 // xorl	%eax, %eax
		code = append(code, 0xb9, 0x08, 0x00, 0x00, 0x00)                               // movl	$0x8, %ecx
		code = append(code, 0xf3, 0x48, 0xab)                                           // rep stosq
		code = append(code, 0x8b, 0x45, 0xa8)                                           // movl	-0x58(%rbp), %eax
		code = append(code, 0x8b, 0x55, 0xa0)                                           // movl	-0x60(%rbp), %edx
		code = append(code, 0x48, 0x0f, 0xae, 0x24, 0x24)                               // xsave64	(%rsp)
		code = append(code, 0xeb, 0x05)                                                 // jmp	0x7d
		code = append(code, 0x48, 0x0f, 0xae, 0x04, 0x24)                               // fxsave64	(%rsp)
		code = append(code, 0xdb, 0xe3)                                                 // fninit
		code = append(code, 0x48, 0x8b, 0x7d, 0xb0)                                     // movq	-0x50(%rbp), %rdi
		code = append(code, 0x48, 0x8b, 0x7f, 0x08)                                     // movq	0x8(%rdi), %rdi
		code = append(code, 0x48, 0x8b, 0x3f)                                           // movq	(%rdi), %rdi
		code = append(code, 0x48, 0xb8, 0x78, 0x56, 0x34, 0x12, 0x00, 0x00, 0x00, 0x00) // movabsq	$0x12345678, %rax
		code = append(code, 0xff, 0xd0)                                                 // callq	*%rax
		code = append(code, 0x48, 0x8b, 0x55, 0xb0)                                     // movq	-0x50(%rbp), %rdx
		code = append(code, 0x48, 0x8b, 0x52, 0x08)                                     // movq	0x8(%rdx), %rdx
		code = append(code, 0x48, 0x03, 0x42, 0x08)                                     // addq	0x8(%rdx), %rax
		code = append(code, 0x64, 0x48, 0x2b, 0x04, 0x25, 0x00, 0x00, 0x00, 0x00)       // subq	%fs:0x0, %rax
		code = append(code, 0x48, 0x89, 0x45, 0xb0)                                     // movq	%rax, -0x50(%rbp)
		code = append(code, 0x83, 0x7d, 0xa8, 0x00)                                     // cmpl	$0x0, -0x58(%rbp)
		code = append(code, 0x74, 0x0d)                                                 // je	0xc2
		code = append(code, 0x8b, 0x45, 0xa8)                                           // movl	-0x58(%rbp), %eax
		code = append(code, 0x8b, 0x55, 0xa0)                                           // movl	-0x60(%rbp), %edx
		code = append(code, 0x48, 0x0f, 0xae, 0x2c, 0x24)                               // xrstor64	(%rsp)
		code = append(code, 0xeb, 0x05)                                                 // jmp	0xc7
		code = append(code, 0x48, 0x0f, 0xae, 0x0c, 0x24)                               // fxrstor64	(%rsp)
		code = append(code, 0x48, 0x8b, 0x45, 0xb0)                                     // movq	-0x50(%rbp), %rax
		code = append(code, 0x48, 0x8d, 0x65, 0xb8)                                     // leaq	-0x48(%rbp), %rsp
		code = append(code, 0x41, 0x5b)                                                 // popq	%r11
		code = append(code, 0x41, 0x5a)                                                 // popq	%r10
		code = append(code, 0x41, 0x59)                                                 // popq	%r9
		code = append(code, 0x41, 0x58)                                                 // popq	%r8
		code = append(code, 0x5f)                                                       // popq	%rdi
		code = append(code, 0x5e)                                                       // popq	%rsi
		code = append(code, 0x5a)                                                       // popq	%rdx
		code = append(code, 0x59)                                                       // popq	%rcx
		code = append(code, 0x5b)                                                       // popq	%rbx
		code = append(code, 0xc9)                                                       // leave
		code = append(code, 0xc3)                                                       // retq
		le.PutUint64(code[140:], uint64(helper))
	case "386":
		code = append(code, 0xf3, 0x0f, 0x1e, 0xfb)                   // endbr32
		code = append(code, 0x55)                                     // pushl	%ebp
		code = append(code, 0x89, 0xe5)                               // movl	%esp, %ebp
		code = append(code, 0x53)                                     // pushl	%ebx
		code = append(code, 0x51)                                     // pushl	%ecx
		code = append(code, 0x52)                                     // pushl	%edx
		code = append(code, 0x56)                                     // pushl	%esi
		code = append(code, 0x57)                                     // pushl	%edi
		code = append(code, 0x50)                                     // pushl	%eax
		code = append(code, 0x6a, 0x00)                               // pushl	$0x0
		code = append(code, 0x6a, 0x00)                               // pushl	$0x0
		code = append(code, 0x83, 0xe4, 0xc0)                         // andl	$-0x40, %esp
		code = append(code, 0xb8, 0x01, 0x00, 0x00, 0x00)             // movl	$0x1, %eax
		code = append(code, 0x0f, 0xa2)                               // cpuid
		code = append(code, 0xf7, 0xc1, 0x00, 0x00, 0x00, 0x08)       // testl	$0x8000000, %ecx
		code = append(code, 0x74, 0x18)                               // je	0x3b
		code = append(code, 0x31, 0xc9)                               // xorl	%ecx, %ecx
		code = append(code, 0x0f, 0x01, 0xd0)                         // xgetbv
		code = append(code, 0x89, 0x45, 0xe4)                         // movl	%eax, -0x1c(%ebp)
		code = append(code, 0x89, 0x55, 0xe0)                         // movl	%edx, -0x20(%ebp)
		code = append(code, 0xb8, 0x0d, 0x00, 0x00, 0x00)             // movl	$0xd, %eax
		code = append(code, 0x31, 0xc9)                               // xorl	%ecx, %ecx
		code = append(code, 0x0f, 0xa2)                               // cpuid
		code = append(code, 0x89, 0xd8)                               // movl	%ebx, %eax
		code = append(code, 0xeb, 0x05)                               // jmp	0x40
		code = append(code, 0xb8, 0x00, 0x02, 0x00, 0x00)             // movl	$0x200, %eax
		code = append(code, 0x83, 0xc0, 0x3f)                         // addl	$0x3f, %eax
		code = append(code, 0x83, 0xe0, 0xc0)                         // andl	$-0x40, %eax
		code = append(code, 0x83, 0xc0, 0x40)                         // addl	$0x40, %eax
		code = append(code, 0x29, 0xc4)                               // subl	%eax, %esp
		code = append(code, 0x8d, 0x74, 0x24, 0x40)                   // leal	0x40(%esp), %esi
		code = append(code, 0x83, 0x7d, 0xe4, 0x00)                   // cmpl	$0x0, -0x1c(%ebp)
		code = append(code, 0x74, 0x1a)                               // je	0x6f
		code = append(code, 0x8d, 0xbe, 0x00, 0x02, 0x00, 0x00)       // leal	0x200(%esi), %edi
		code = append(code, 0x31, 0xc0)                               // xorl	%eax, %eax
		code = append(code, 0xb9, 0x10, 0x00, 0x00, 0x00)             // movl	$0x10, %ecx
		code = append(code, 0xf3, 0xab)                               // rep stosl
		code = append(code, 0x8b, 0x45, 0xe4)                         // movl	-0x1c(%ebp), %eax
		code = append(code, 0x8b, 0x55, 0xe0)                         // movl	-0x20(%ebp), %edx
		code = append(code, 0x0f, 0xae, 0x26)                         // xsave	(%esi)
		code = append(code, 0xeb, 0x03)                               // jmp	0x72
		code = append(code, 0x0f, 0xae, 0x06)                         // fxsave	(%esi)
		code = append(code, 0xdb, 0xe3)                               // fninit
		code = append(code, 0x8b, 0x45, 0xe8)                         // movl	-0x18(%ebp), %eax
		code = append(code, 0x8b, 0x40, 0x04)                         // movl	0x4(%eax), %eax
		code = append(code, 0x8b, 0x00)                               // movl	(%eax), %eax
		code = append(code, 0x89, 0x04, 0x24)                         // movl	%eax, (%esp)
		code = append(code, 0xb8, 0x78, 0x56, 0x34, 0x12)             // movl	$0x12345678, %eax
		code = append(code, 0xff, 0xd0)                               // calll	*%eax
		code = append(code, 0x8b, 0x55, 0xe8)                         // movl	-0x18(%ebp), %edx
		code = append(code, 0x8b, 0x52, 0x04)                         // movl	0x4(%edx), %edx
		code = append(code, 0x03, 0x42, 0x04)                         // addl	0x4(%edx), %eax
		code = append(code, 0x65, 0x2b, 0x05, 0x00, 0x00, 0x00, 0x00) // subl	%gs:0x0, %eax
		code = append(code, 0x89, 0x45, 0xe8)                         // movl	%eax, -0x18(%ebp)
		code = append(code, 0x83, 0x7d, 0xe4, 0x00)                   // cmpl	$0x0, -0x1c(%ebp)
		code = append(code, 0x74, 0x0b)                               // je	0xaa
		code = append(code, 0x8b, 0x45, 0xe4)                         // movl	-0x1c(%ebp), %eax
		code = append(code, 0x8b, 0x55, 0xe0)                         // movl	-0x20(%ebp), %edx
		code = append(code, 0x0f, 0xae, 0x2e)                         // xrstor	(%esi)
		code = append(code, 0xeb, 0x03)                               // jmp	0xad
		code = append(code, 0x0f, 0xae, 0x0e)                         // fxrstor	(%esi)
		code = append(code, 0x8b, 0x45, 0xe8)                         // movl	-0x18(%ebp), %eax
		code = append(code, 0x8d, 0x65, 0xec)                         // leal	-0x14(%ebp), %esp
		code = append(code, 0x5f)                                     // popl	%edi
		code = append(code, 0x5e)                                     // popl	%esi
		code = append(code, 0x5a)                                     // popl	%edx
		code = append(code, 0x59)                                     // popl	%ecx
		code = append(code, 0x5b)                                     // popl	%ebx
		code = append(code, 0xc9)                                     // leave
		code = append(code, 0xc3)                                     // retl
		le.PutUint32(code[128:], uint32(helper))
	default:
		return nil, fmt.Errorf("native ELF TLSDESC is unsupported on %s", arch)
	}
	return code, nil
}
