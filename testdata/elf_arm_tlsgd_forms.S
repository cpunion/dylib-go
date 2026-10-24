// AAELF64 GD address forms which current GCC/Clang C fixtures do not emit.
// Explicit relocations also work with Clang assemblers lacking GNU TLS syntax.
.text
.globl tls_shared
.type tls_shared, %tls_object
.macro tls_index symbol
#if DYLIB_TLSGD_FORM == 0
    adr x0, .
    .reloc .-4, R_AARCH64_TLSGD_ADR_PREL21, \symbol
#elif DYLIB_TLSGD_FORM == 1
    movz x0, #0, lsl #16
    .reloc .-4, R_AARCH64_TLSGD_MOVW_G1, \symbol
    movk x0, #0
    .reloc .-4, R_AARCH64_TLSGD_MOVW_G0_NC, \symbol
    adrp x1, _GLOBAL_OFFSET_TABLE_
    add x1, x1, :lo12:_GLOBAL_OFFSET_TABLE_
    add x0, x1, x0
#else
    adrp x0, 0
    .reloc .-4, R_AARCH64_TLSGD_ADR_PAGE21, \symbol
    add x0, x0, #0
    .reloc .-4, R_AARCH64_TLSGD_ADD_LO12_NC, \symbol
#endif
    bl __tls_get_addr
.endm
.p2align 2
.globl tls_form_step
.type tls_form_step, %function
tls_form_step:
    stp x29, x30, [sp, #-48]!
    mov x29, sp
    stp x19, x20, [sp, #16]
    str x21, [sp, #32]
    mov w19, w0
    mov w20, w1
    tls_index tls_shared
    mov x21, x0
    bl tls_read
    ldr w2, [x21]
    cmp w0, w2
    b.ne .Lfailure
    tls_index .Lzero
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
.size tls_form_step, .-tls_form_step
.section .tbss,"awT",%nobits
.p2align 6
.type .Lzero, %tls_object
.Lzero:
    .zero 64
.size .Lzero, 64
.section .note.GNU-stack,"",%progbits
