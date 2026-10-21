.text
.globl _tls_registers
#if defined(__aarch64__)
.p2align 2
_tls_registers:
.cfi_startproc
stp x29, x30, [sp, #-160]!
mov x29, sp
.cfi_def_cfa w29, 160
.cfi_offset w29, -160
.cfi_offset w30, -152
stp q8, q9, [sp, #32]
stp q10, q11, [sp, #64]
stp q12, q13, [sp, #96]
stp q14, q15, [sp, #128]
adrp x0, _tls_shared@TLVPPAGE
ldr x0, [x0, _tls_shared@TLVPPAGEOFF]
ldr x16, [x0]
.irp n,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15
mov x\n, #42
.endr
.irp n,0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31
movi v\n\().16b, #0x5a
.endr
blr x16
.irp n,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15
cmp x\n, #42
b.ne Lfail
.endr
movz x17, #0x5a5a
movk x17, #0x5a5a, lsl #16
movk x17, #0x5a5a, lsl #32
movk x17, #0x5a5a, lsl #48
.irp n,0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31
umov x16, v\n\().d[0]
cmp x16, x17
b.ne Lfail
umov x16, v\n\().d[1]
cmp x16, x17
b.ne Lfail
.endr
cbz x0, Lfail
mov w0, #42
b Ldone
Lfail:
mov w0, #0
Ldone:
ldp q8, q9, [sp, #32]
ldp q10, q11, [sp, #64]
ldp q12, q13, [sp, #96]
ldp q14, q15, [sp, #128]
ldp x29, x30, [sp], #160
ret
.cfi_endproc
#else
_tls_registers:
.cfi_startproc
pushq %rbp
pushq %rbx
pushq %r12
pushq %r13
pushq %r14
pushq %r15
subq $8, %rsp
.cfi_def_cfa_offset 64
movq _tls_shared@TLVP(%rip), %rdi
.irp reg,rcx,rdx,rsi,r8,r9,r10,r11,rbp,rbx,r12,r13,r14,r15
movq $42, %\reg
.endr
callq *(%rdi)
.irp reg,rcx,rdx,rsi,r8,r9,r10,r11,rbp,rbx,r12,r13,r14,r15
cmpq $42, %\reg
jne Lfail
.endr
testq %rax, %rax
je Lfail
movl $42, %eax
jmp Ldone
Lfail:
xorl %eax, %eax
Ldone:
addq $8, %rsp
popq %r15
popq %r14
popq %r13
popq %r12
popq %rbx
popq %rbp
retq
.cfi_endproc
#endif
