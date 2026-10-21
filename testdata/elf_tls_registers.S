.text
.globl tls_registers
.p2align 2
tls_registers:
.cfi_startproc
stp x29, x30, [sp, #-160]!
mov x29, sp
.cfi_def_cfa x29, 160
.cfi_offset x29, -160
.cfi_offset x30, -152
stp q8, q9, [sp, #32]
stp q10, q11, [sp, #64]
stp q12, q13, [sp, #96]
stp q14, q15, [sp, #128]
adrp x0, :tlsdesc:tls_shared
ldr x1, [x0, :tlsdesc_lo12:tls_shared]
add x0, x0, :tlsdesc_lo12:tls_shared
str x1, [sp, #16]
.irp n,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18
mov x\n, #42
.endr
.irp n,0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31
movi v\n\().16b, #0x5a
.endr
.tlsdesccall tls_shared
blr x1
ldr x0, [sp, #16]
cmp x0, x1
b.ne Lfail
.irp n,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18
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
.section .note.GNU-stack,"",@progbits
