.text
#if defined(__aarch64__)
.p2align 2
reference_entry:
stp x29, x30, [sp, #-16]!
mov x29, sp
sub sp, sp, #672
stp x1, x2, [sp]
stp x3, x4, [sp, #16]
stp x5, x6, [sp, #32]
stp x7, x8, [sp, #48]
stp x9, x10, [sp, #64]
stp x11, x12, [sp, #80]
stp x13, x14, [sp, #96]
stp x15, x16, [sp, #112]
stp x17, x18, [sp, #128]
str x0, [sp, #144]
stp q0, q1, [sp, #160]
stp q2, q3, [sp, #192]
stp q4, q5, [sp, #224]
stp q6, q7, [sp, #256]
stp q8, q9, [sp, #288]
stp q10, q11, [sp, #320]
stp q12, q13, [sp, #352]
stp q14, q15, [sp, #384]
stp q16, q17, [sp, #416]
stp q18, q19, [sp, #448]
stp q20, q21, [sp, #480]
stp q22, q23, [sp, #512]
stp q24, q25, [sp, #544]
stp q26, q27, [sp, #576]
stp q28, q29, [sp, #608]
stp q30, q31, [sp, #640]
ldr x0, [x0, #8]
ldr x0, [x0]
movz x16, #0x5678
movk x16, #0x1234, lsl #16
movk x16, #0, lsl #32
movk x16, #0, lsl #48
blr x16
ldr x16, [sp, #144]
ldr x16, [x16, #8]
ldr x16, [x16, #8]
add x0, x0, x16
mrs x16, TPIDR_EL0
sub x0, x0, x16
ldp q0, q1, [sp, #160]
ldp q2, q3, [sp, #192]
ldp q4, q5, [sp, #224]
ldp q6, q7, [sp, #256]
ldp q8, q9, [sp, #288]
ldp q10, q11, [sp, #320]
ldp q12, q13, [sp, #352]
ldp q14, q15, [sp, #384]
ldp q16, q17, [sp, #416]
ldp q18, q19, [sp, #448]
ldp q20, q21, [sp, #480]
ldp q22, q23, [sp, #512]
ldp q24, q25, [sp, #544]
ldp q26, q27, [sp, #576]
ldp q28, q29, [sp, #608]
ldp q30, q31, [sp, #640]
ldp x1, x2, [sp]
ldp x3, x4, [sp, #16]
ldp x5, x6, [sp, #32]
ldp x7, x8, [sp, #48]
ldp x9, x10, [sp, #64]
ldp x11, x12, [sp, #80]
ldp x13, x14, [sp, #96]
ldp x15, x16, [sp, #112]
ldp x17, x18, [sp, #128]
add sp, sp, #672
ldp x29, x30, [sp], #16
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
