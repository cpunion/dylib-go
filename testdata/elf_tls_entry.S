.text
#if defined(__aarch64__)
.p2align 2
reference_entry:
stp x29, x30, [sp, #-32]!
mov x29, sp
str x19, [sp, #16]
mov x19, x0
ldr x0, [x0]
movz x16, #0x5678
movk x16, #0x1234, lsl #16
movk x16, #0, lsl #32
movk x16, #0, lsl #48
blr x16
ldr x16, [x19, #8]
add x0, x0, x16
ldr x19, [sp, #16]
ldp x29, x30, [sp], #32
ret
#elif defined(__x86_64__)
reference_entry:
pushq %rdi
movq (%rdi), %rdi
movabsq $0x12345678, %rax
callq *%rax
popq %rdx
addq 8(%rdx), %rax
retq
#else
reference_entry:
pushl %ebp
movl %esp, %ebp
andl $-16, %esp
subl $16, %esp
movl %eax, 4(%esp)
movl (%eax), %eax
movl %eax, (%esp)
movl $0x12345678, %eax
calll *%eax
movl 4(%esp), %edx
addl 4(%edx), %eax
movl %ebp, %esp
popl %ebp
retl
#endif
.section .note.GNU-stack,"",@progbits
