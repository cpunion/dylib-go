// Independent assembler reference for the Go-generated TLSDESC adapters.
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
#else
reference_entry:
#if defined(__x86_64__)
endbr64
pushq %rbp
movq %rsp, %rbp
pushq %rbx
pushq %rcx
pushq %rdx
pushq %rsi
pushq %rdi
pushq %r8
pushq %r9
pushq %r10
pushq %r11
pushq %rax
pushq $0
pushq $0
andq $-64, %rsp
#else
endbr32
pushl %ebp
movl %esp, %ebp
pushl %ebx
pushl %ecx
pushl %edx
pushl %esi
pushl %edi
pushl %eax
pushl $0
pushl $0
andl $-64, %esp
#endif
movl $1, %eax
cpuid
// OSXSAVE guarantees XGETBV and the enabled user-state save area.
testl $0x08000000, %ecx
jz .Lfxsize
xorl %ecx, %ecx
xgetbv
#if defined(__x86_64__)
movl %eax, -88(%rbp)
movl %edx, -96(%rbp)
#else
movl %eax, -28(%ebp)
movl %edx, -32(%ebp)
#endif
movl $13, %eax
xorl %ecx, %ecx
cpuid
movl %ebx, %eax
jmp .Lallocate
.Lfxsize:
movl $512, %eax
.Lallocate:
addl $63, %eax
andl $-64, %eax
#if defined(__x86_64__)
subq %rax, %rsp
cmpl $0, -88(%rbp)
je .Lfxsave
// Clear the complete XSAVE header, including reserved words.
leaq 512(%rsp), %rdi
xorl %eax, %eax
movl $8, %ecx
rep stosq
movl -88(%rbp), %eax
movl -96(%rbp), %edx
xsave64 (%rsp)
jmp .Lcall
.Lfxsave:
fxsave64 (%rsp)
.Lcall:
fninit
movq -80(%rbp), %rdi
movq 8(%rdi), %rdi
movq (%rdi), %rdi
movabsq $0x12345678, %rax
callq *%rax
movq -80(%rbp), %rdx
movq 8(%rdx), %rdx
addq 8(%rdx), %rax
subq %fs:0, %rax
movq %rax, -80(%rbp)
cmpl $0, -88(%rbp)
je .Lfxrestore
movl -88(%rbp), %eax
movl -96(%rbp), %edx
xrstor64 (%rsp)
jmp .Lreturn
.Lfxrestore:
fxrstor64 (%rsp)
.Lreturn:
movq -80(%rbp), %rax
leaq -72(%rbp), %rsp
popq %r11
popq %r10
popq %r9
popq %r8
popq %rdi
popq %rsi
popq %rdx
popq %rcx
popq %rbx
leave
retq
#else
// Keep a separate aligned outgoing cdecl argument area below the state.
addl $64, %eax
subl %eax, %esp
leal 64(%esp), %esi
cmpl $0, -28(%ebp)
je .Lfxsave
leal 512(%esi), %edi
xorl %eax, %eax
movl $16, %ecx
rep stosl
movl -28(%ebp), %eax
movl -32(%ebp), %edx
xsave (%esi)
jmp .Lcall
.Lfxsave:
fxsave (%esi)
.Lcall:
fninit
movl -24(%ebp), %eax
movl 4(%eax), %eax
movl (%eax), %eax
movl %eax, (%esp)
movl $0x12345678, %eax
calll *%eax
movl -24(%ebp), %edx
movl 4(%edx), %edx
addl 4(%edx), %eax
subl %gs:0, %eax
movl %eax, -24(%ebp)
cmpl $0, -28(%ebp)
je .Lfxrestore
movl -28(%ebp), %eax
movl -32(%ebp), %edx
xrstor (%esi)
jmp .Lreturn
.Lfxrestore:
fxrstor (%esi)
.Lreturn:
movl -24(%ebp), %eax
leal -20(%ebp), %esp
popl %edi
popl %esi
popl %edx
popl %ecx
popl %ebx
leave
retl
#endif
#endif
.section .note.GNU-stack,"",@progbits
