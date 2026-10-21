#if defined(__aarch64__)
.text
.p2align 2
_reference_entry:
stp x29, x30, [sp, #-16]!
mov x29, sp
sub sp, sp, #640
stp x1, x2, [sp]
stp x3, x4, [sp, #16]
stp x5, x6, [sp, #32]
stp x7, x8, [sp, #48]
stp x9, x10, [sp, #64]
stp x11, x12, [sp, #80]
stp x13, x14, [sp, #96]
str x15, [sp, #112]
str x0, [sp, #120]
stp q0, q1, [sp, #128]
stp q2, q3, [sp, #160]
stp q4, q5, [sp, #192]
stp q6, q7, [sp, #224]
stp q8, q9, [sp, #256]
stp q10, q11, [sp, #288]
stp q12, q13, [sp, #320]
stp q14, q15, [sp, #352]
stp q16, q17, [sp, #384]
stp q18, q19, [sp, #416]
stp q20, q21, [sp, #448]
stp q22, q23, [sp, #480]
stp q24, q25, [sp, #512]
stp q26, q27, [sp, #544]
stp q28, q29, [sp, #576]
stp q30, q31, [sp, #608]
ldr x0, [x0, #8]
movz x16, #0x5678
movk x16, #0x1234, lsl #16
movk x16, #0, lsl #32
movk x16, #0, lsl #48
blr x16
ldr x16, [sp, #120]
ldr x16, [x16, #16]
add x0, x0, x16
ldp q0, q1, [sp, #128]
ldp q2, q3, [sp, #160]
ldp q4, q5, [sp, #192]
ldp q6, q7, [sp, #224]
ldp q8, q9, [sp, #256]
ldp q10, q11, [sp, #288]
ldp q12, q13, [sp, #320]
ldp q14, q15, [sp, #352]
ldp q16, q17, [sp, #384]
ldp q18, q19, [sp, #416]
ldp q20, q21, [sp, #448]
ldp q22, q23, [sp, #480]
ldp q24, q25, [sp, #512]
ldp q26, q27, [sp, #544]
ldp q28, q29, [sp, #576]
ldp q30, q31, [sp, #608]
ldp x1, x2, [sp]
ldp x3, x4, [sp, #16]
ldp x5, x6, [sp, #32]
ldp x7, x8, [sp, #48]
ldp x9, x10, [sp, #64]
ldp x11, x12, [sp, #80]
ldp x13, x14, [sp, #96]
ldr x15, [sp, #112]
add sp, sp, #640
ldp x29, x30, [sp], #16
ret
#else
.text
_reference_entry:
pushq %rcx
pushq %rdx
pushq %rsi
pushq %r8
pushq %r9
pushq %r10
pushq %r11
pushq %rdi
subq $8, %rsp
movq 8(%rdi), %rdi
movabsq $0x12345678, %rax
callq *%rax
movq 8(%rsp), %rdi
addq 16(%rdi), %rax
addq $8, %rsp
popq %rdi
popq %r11
popq %r10
popq %r9
popq %r8
popq %rsi
popq %rdx
popq %rcx
retq
#endif
