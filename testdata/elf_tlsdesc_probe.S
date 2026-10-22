// A hostile C allocator checks the adapter independently of its real allocator.
// Wide probes run only after CPUID/XGETBV validate the CPU and OS capabilities.
#ifndef DYLIB_TLS_PROBE_WIDTH
#define DYLIB_TLS_PROBE_WIDTH 128
#endif
.text
.globl tlsdesc_probe
.type tlsdesc_probe,@function
tlsdesc_probe:
#if defined(__x86_64__)
pushq %rbp
pushq %rbx
pushq %r12
pushq %r13
pushq %r14
pushq %r15
subq $648, %rsp
fxsave64 128(%rsp)
movq %rdi, (%rsp)
leaq 16(%rsp), %rax
movq %rax, 8(%rsp)
leaq 40(%rsp), %rax
movq %rax, 16(%rsp)
movq $0, 24(%rsp)
movl $7, 40(%rsp)
#else
pushl %ebp
pushl %ebx
pushl %esi
pushl %edi
subl $652, %esp
fxsave 128(%esp)
movl 672(%esp), %eax
movl %eax, (%esp)
leal 8(%esp), %eax
movl %eax, 4(%esp)
leal 40(%esp), %eax
movl %eax, 8(%esp)
movl $0, 12(%esp)
movl $7, 40(%esp)
#endif
#if defined(__x86_64__)
.irp n,0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15
#else
.irp n,0,1,2,3,4,5,6,7
#endif
#if DYLIB_TLS_PROBE_WIDTH == 512
vpternlogd $255, %zmm\n, %zmm\n, %zmm\n
#elif DYLIB_TLS_PROBE_WIDTH == 256
vxorps %ymm\n, %ymm\n, %ymm\n
vcmpps $0, %ymm\n, %ymm\n, %ymm\n
#else
pcmpeqd %xmm\n, %xmm\n
#endif
.endr
#if DYLIB_TLS_PROBE_WIDTH == 512 && defined(__x86_64__)
.irp n,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31
vpternlogd $255, %zmm\n, %zmm\n, %zmm\n
.endr
#endif
#if DYLIB_TLS_PROBE_WIDTH == 512
.irp n,0,1,2,3,4,5,6,7
kxnorw %k\n, %k\n, %k\n
.endr
#endif
.rept 8
fld1
.endr
#if defined(__x86_64__)
movq $3, %rbx
movq $4, %rcx
movq $5, %rdx
movq $6, %rsi
movq $7, %rdi
movq $8, %rbp
movq $9, %r8
movq $10, %r9
movq $11, %r10
movq $12, %r11
movq $13, %r12
movq $14, %r13
movq $15, %r14
movq $16, %r15
movq %rsp, %rax
callq *(%rax)
addq %fs:0, %rax
cmpl $7, (%rax)
jne .Lfail
cmpq $3, %rbx
jne .Lfail
cmpq $4, %rcx
jne .Lfail
cmpq $5, %rdx
jne .Lfail
cmpq $6, %rsi
jne .Lfail
cmpq $7, %rdi
jne .Lfail
cmpq $8, %rbp
jne .Lfail
cmpq $9, %r8
jne .Lfail
cmpq $10, %r9
jne .Lfail
cmpq $11, %r10
jne .Lfail
cmpq $12, %r11
jne .Lfail
cmpq $13, %r12
jne .Lfail
cmpq $14, %r13
jne .Lfail
cmpq $15, %r14
jne .Lfail
cmpq $16, %r15
jne .Lfail
#else
movl $3, %ebx
movl $4, %ecx
movl $5, %edx
movl $6, %esi
movl $7, %edi
movl $8, %ebp
movl %esp, %eax
calll *(%eax)
addl %gs:0, %eax
cmpl $7, (%eax)
jne .Lfail
cmpl $3, %ebx
jne .Lfail
cmpl $4, %ecx
jne .Lfail
cmpl $5, %edx
jne .Lfail
cmpl $6, %esi
jne .Lfail
cmpl $7, %edi
jne .Lfail
cmpl $8, %ebp
jne .Lfail
#endif
#if DYLIB_TLS_PROBE_WIDTH == 512
.irp n,0,1,2,3,4,5,6,7
kmovw %k\n, %eax
cmpl $65535, %eax
jne .Lfail
.endr
#endif
#if defined(__x86_64__)
.irp n,0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15
#else
.irp n,0,1,2,3,4,5,6,7
#endif
#if DYLIB_TLS_PROBE_WIDTH == 512
vptestmd %zmm\n, %zmm\n, %k1
kmovw %k1, %eax
#elif DYLIB_TLS_PROBE_WIDTH == 256
pmovmskb %xmm\n, %eax
cmpl $65535, %eax
jne .Lfail
vextractf128 $1, %ymm\n, %xmm\n
pmovmskb %xmm\n, %eax
#else
pmovmskb %xmm\n, %eax
#endif
cmpl $65535, %eax
jne .Lfail
.endr
#if DYLIB_TLS_PROBE_WIDTH == 512 && defined(__x86_64__)
.irp n,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31
vptestmd %zmm\n, %zmm\n, %k1
kmovw %k1, %eax
cmpl $65535, %eax
jne .Lfail
.endr
#endif
.rept 8
#if defined(__x86_64__)
fstpl 64(%rsp)
cmpl $0, 64(%rsp)
jne .Lfail
cmpl $0x3ff00000, 68(%rsp)
#else
fstpl 64(%esp)
cmpl $0, 64(%esp)
jne .Lfail
cmpl $0x3ff00000, 68(%esp)
#endif
jne .Lfail
.endr
xorl %eax, %eax
jmp .Lfinish
.Lfail:
movl $-1, %eax
.Lfinish:
#if defined(__x86_64__)
movl %eax, .Lresult(%rip)
fxrstor64 128(%rsp)
addq $648, %rsp
popq %r15
popq %r14
popq %r13
popq %r12
popq %rbx
popq %rbp
retq
#else
call 1f
1: popl %edx
movl %eax, .Lresult-1b(%edx)
fxrstor 128(%esp)
addl $652, %esp
popl %edi
popl %esi
popl %ebx
popl %ebp
retl
#endif
.size tlsdesc_probe, .-tlsdesc_probe
.globl tlsdesc_probe_result
.type tlsdesc_probe_result,@function
tlsdesc_probe_result:
#if defined(__x86_64__)
movl .Lresult(%rip), %eax
retq
#else
call 1f
1: popl %eax
movl .Lresult-1b(%eax), %eax
retl
#endif
.size tlsdesc_probe_result, .-tlsdesc_probe_result
.globl tlsdesc_clobber
.type tlsdesc_clobber,@function
tlsdesc_clobber:
#if defined(__x86_64__)
movq %rdi, %rax
xorq %rcx, %rcx
xorq %rdx, %rdx
xorq %rsi, %rsi
xorq %rdi, %rdi
xorq %r8, %r8
xorq %r9, %r9
xorq %r10, %r10
xorq %r11, %r11
#else
movl 4(%esp), %eax
xorl %ecx, %ecx
xorl %edx, %edx
#endif
#if defined(__x86_64__)
.irp n,0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15
#else
.irp n,0,1,2,3,4,5,6,7
#endif
#if DYLIB_TLS_PROBE_WIDTH == 512
vpxord %zmm\n, %zmm\n, %zmm\n
#elif DYLIB_TLS_PROBE_WIDTH == 256
vxorps %ymm\n, %ymm\n, %ymm\n
#else
pxor %xmm\n, %xmm\n
#endif
.endr
#if DYLIB_TLS_PROBE_WIDTH == 512 && defined(__x86_64__)
.irp n,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31
vpxord %zmm\n, %zmm\n, %zmm\n
.endr
#endif
#if DYLIB_TLS_PROBE_WIDTH == 512
.irp n,0,1,2,3,4,5,6,7
kxorw %k\n, %k\n, %k\n
.endr
#endif
fninit
ret
.size tlsdesc_clobber, .-tlsdesc_clobber
.globl tlsdesc_probe_supported
.type tlsdesc_probe_supported,@function
tlsdesc_probe_supported:
#if DYLIB_TLS_PROBE_WIDTH == 128
movl $1, %eax
ret
#else
#if defined(__x86_64__)
pushq %rbx
#else
pushl %ebx
#endif
movl $1, %eax
cpuid
andl $0x18000000, %ecx
cmpl $0x18000000, %ecx
jne .Lunsupported
xorl %ecx, %ecx
xgetbv
#if DYLIB_TLS_PROBE_WIDTH == 512
andl $0xe6, %eax
cmpl $0xe6, %eax
jne .Lunsupported
movl $7, %eax
xorl %ecx, %ecx
cpuid
testl $0x10000, %ebx
jz .Lunsupported
#else
andl $6, %eax
cmpl $6, %eax
jne .Lunsupported
#endif
movl $1, %eax
jmp .Lsupported
.Lunsupported:
xorl %eax, %eax
.Lsupported:
#if defined(__x86_64__)
popq %rbx
#else
popl %ebx
#endif
ret
#endif
.size tlsdesc_probe_supported, .-tlsdesc_probe_supported
.data
.p2align 2
.Lresult: .long -2
.section .note.GNU-stack,"",@progbits
