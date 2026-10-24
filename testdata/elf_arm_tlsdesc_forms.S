#include "elf_arm_tlsdesc_address.inc"
// Independent AAELF64 descriptor address and annotation sequences.
.text
.globl tls_shared
.hidden tls_shared
.type tls_shared, %tls_object
.macro tls_address symbol
    tls_desc_pointer \symbol
    .tlsdesccall \symbol
    blr x1
    mrs x1, TPIDR_EL0
    add x0, x0, x1
.endm
.p2align 2
.globl tls_desc_step
.type tls_desc_step, %function
tls_desc_step:
    stp x29, x30, [sp, #-48]!
    mov x29, sp
    stp x19, x20, [sp, #16]
    str x21, [sp, #32]
    mov w19, w0
    mov w20, w1
    tls_address tls_shared
    mov x21, x0
    bl tls_read
    ldr w2, [x21]
    cmp w0, w2
    b.ne .Lfailure
    tls_address tls_desc_zero
    tst x0, #63
    b.ne .Lfailure
    ldr w2, [x0]
    ldr w3, [x21]
    sub w4, w3, #7
    cmp w2, w4
    b.ne .Lfailure
    add w2, w2, #1
    str w2, [x0]
    add w2, w3, #1
    str w2, [x21]
    add w0, w3, w19
    add w0, w0, w20
    b .Ldone
.Lfailure:
    mov w0, #-1
.Ldone:
    ldr x21, [sp, #32]
    ldp x19, x20, [sp, #16]
    ldp x29, x30, [sp], #48
    ret
.size tls_desc_step, .-tls_desc_step
.section .tbss,"awT",%nobits
.p2align 6
.type tls_desc_zero, %tls_object
tls_desc_zero:
.zero 64
.size tls_desc_zero, 64
.section .note.GNU-stack,"",%progbits
