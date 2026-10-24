// Independent AAELF64 TLSLD/DTPREL fixture, also linked with GNU ld.
.text
.macro module_index symbol
#if DYLIB_TLSLD_FORM == 0
    adr x0, .
    .reloc .-4, R_AARCH64_TLSLD_ADR_PREL21, \symbol
#elif DYLIB_TLSLD_FORM == 1
    adrp x0, 0
    .reloc .-4, R_AARCH64_TLSLD_ADR_PAGE21, \symbol
    add x0, x0, #0
    .reloc .-4, R_AARCH64_TLSLD_ADD_LO12_NC, \symbol
#elif DYLIB_TLSLD_FORM == 2
    movz x0, #0, lsl #16
    .reloc .-4, R_AARCH64_TLSLD_MOVW_G1, \symbol
    movk x0, #0
    .reloc .-4, R_AARCH64_TLSLD_MOVW_G0_NC, \symbol
    adrp x1, _GLOBAL_OFFSET_TABLE_
    add x1, x1, :lo12:_GLOBAL_OFFSET_TABLE_
    add x0, x1, x0
#else
    ldr q0, .
    .reloc .-4, R_AARCH64_TLSLD_LD_PREL19, \symbol
    str q0, [sp, #64]
    add x0, sp, #64
#endif
    bl __tls_get_addr
.endm
.globl tls_shared
.hidden tls_shared
.type tls_shared, %tls_object
.globl tls_ld_step
.type tls_ld_step, %function
.p2align 2
tls_ld_step:
    stp x29, x30, [sp, #-80]!
    mov x29, sp
    stp x19, x20, [sp, #16]
    stp x21, x22, [sp, #32]
    stp x23, x24, [sp, #48]
    mov w19, w0
    mov w20, w1
    module_index tls_ld_anchor
    mov x21, x0
    module_index tls_ld_counter
    cmp x0, x21
    b.ne .Lfailure
    movz x23, #0, lsl #32
    .reloc .-4, R_AARCH64_TLSLD_MOVW_DTPREL_G2, tls_shared
    movk x23, #0, lsl #16
    .reloc .-4, R_AARCH64_TLSLD_MOVW_DTPREL_G1_NC, tls_shared
    movk x23, #0
    .reloc .-4, R_AARCH64_TLSLD_MOVW_DTPREL_G0_NC, tls_shared
    add x23, x21, x23
    movn x24, #0, lsl #16
    .reloc .-4, R_AARCH64_TLSLD_MOVW_DTPREL_G1, tls_ld_counter
    movk x24, #0
    .reloc .-4, R_AARCH64_TLSLD_MOVW_DTPREL_G0_NC, tls_ld_counter
    add x24, x21, x24
    add x2, x21, #0, lsl #12
    .reloc .-4, R_AARCH64_TLSLD_ADD_DTPREL_HI12, tls_ld_counter
    add x2, x2, #0
    .reloc .-4, R_AARCH64_TLSLD_ADD_DTPREL_LO12_NC, tls_ld_counter
    cmp x2, x24
    b.ne .Lfailure
    movn x2, #0
    .reloc .-4, R_AARCH64_TLSLD_MOVW_DTPREL_G0, tls_ld_b
    add x2, x21, x2
    add x3, x21, #0
    .reloc .-4, R_AARCH64_TLSLD_ADD_DTPREL_LO12, tls_ld_b
    cmp x2, x3
    b.ne .Lfailure
    ldrb w2, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST8_DTPREL_LO12, tls_ld_b
    ldr w3, =0x12
    cmp w2, w3
    b.ne .Lfailure
    strb w2, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST8_DTPREL_LO12, tls_ld_b
    add x22, x21, #0, lsl #12
    .reloc .-4, R_AARCH64_TLSLD_ADD_DTPREL_HI12, tls_ld_zb
    ldrb w2, [x22]
    .reloc .-4, R_AARCH64_TLSLD_LDST8_DTPREL_LO12_NC, tls_ld_zb
    cbnz w2, .Lfailure
    mov w2, #1
    strb w2, [x22]
    .reloc .-4, R_AARCH64_TLSLD_LDST8_DTPREL_LO12_NC, tls_ld_zb
    ldrh w2, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST16_DTPREL_LO12, tls_ld_h
    ldr w3, =0x1234
    cmp w2, w3
    b.ne .Lfailure
    strh w2, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST16_DTPREL_LO12, tls_ld_h
    add x22, x21, #0, lsl #12
    .reloc .-4, R_AARCH64_TLSLD_ADD_DTPREL_HI12, tls_ld_zh
    ldrh w2, [x22]
    .reloc .-4, R_AARCH64_TLSLD_LDST16_DTPREL_LO12_NC, tls_ld_zh
    cbnz w2, .Lfailure
    mov w2, #1
    strh w2, [x22]
    .reloc .-4, R_AARCH64_TLSLD_LDST16_DTPREL_LO12_NC, tls_ld_zh
    ldr w2, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST32_DTPREL_LO12, tls_ld_w
    ldr w3, =0x12345678
    cmp w2, w3
    b.ne .Lfailure
    str w2, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST32_DTPREL_LO12, tls_ld_w
    add x22, x21, #0, lsl #12
    .reloc .-4, R_AARCH64_TLSLD_ADD_DTPREL_HI12, tls_ld_zw
    ldr w2, [x22]
    .reloc .-4, R_AARCH64_TLSLD_LDST32_DTPREL_LO12_NC, tls_ld_zw
    cbnz w2, .Lfailure
    mov w2, #1
    str w2, [x22]
    .reloc .-4, R_AARCH64_TLSLD_LDST32_DTPREL_LO12_NC, tls_ld_zw
    ldr x2, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST64_DTPREL_LO12, tls_ld_d
    ldr x3, =0x123456789abcdef0
    cmp x2, x3
    b.ne .Lfailure
    str x2, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST64_DTPREL_LO12, tls_ld_d
    add x22, x21, #0, lsl #12
    .reloc .-4, R_AARCH64_TLSLD_ADD_DTPREL_HI12, tls_ld_zd
    ldr x2, [x22]
    .reloc .-4, R_AARCH64_TLSLD_LDST64_DTPREL_LO12_NC, tls_ld_zd
    cbnz x2, .Lfailure
    mov x2, #1
    str x2, [x22]
    .reloc .-4, R_AARCH64_TLSLD_LDST64_DTPREL_LO12_NC, tls_ld_zd
#ifdef DYLIB_TLSLD_SYSTEM_CONTROL
    add x4, x21, #0
    .reloc .-4, R_AARCH64_TLSLD_ADD_DTPREL_LO12, tls_ld_q
    ldr q0, [x4]
#else
    ldr q0, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST128_DTPREL_LO12, tls_ld_q
#endif
    umov x2, v0.d[0]
    cmp x2, #20
    b.ne .Lfailure
    umov x2, v0.d[1]
    cmp x2, #22
    b.ne .Lfailure
#ifdef DYLIB_TLSLD_SYSTEM_CONTROL
    add x4, x21, #0
    .reloc .-4, R_AARCH64_TLSLD_ADD_DTPREL_LO12, tls_ld_q
    str q0, [x4]
#else
    str q0, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST128_DTPREL_LO12, tls_ld_q
#endif
    add x22, x21, #0, lsl #12
    .reloc .-4, R_AARCH64_TLSLD_ADD_DTPREL_HI12, tls_ld_zq
#ifdef DYLIB_TLSLD_SYSTEM_CONTROL
    add x4, x22, #0
    .reloc .-4, R_AARCH64_TLSLD_ADD_DTPREL_LO12_NC, tls_ld_zq
    ldr q0, [x4]
#else
    ldr q0, [x22]
    .reloc .-4, R_AARCH64_TLSLD_LDST128_DTPREL_LO12_NC, tls_ld_zq
#endif
    umov x2, v0.d[0]
    umov x3, v0.d[1]
    orr x2, x2, x3
    cbnz x2, .Lfailure
    mov x2, #1
    ins v0.d[0], x2
    ins v0.d[1], x2
#ifdef DYLIB_TLSLD_SYSTEM_CONTROL
    add x4, x22, #0
    .reloc .-4, R_AARCH64_TLSLD_ADD_DTPREL_LO12_NC, tls_ld_zq
    str q0, [x4]
#else
    str q0, [x22]
    .reloc .-4, R_AARCH64_TLSLD_LDST128_DTPREL_LO12_NC, tls_ld_zq
#endif
    ldr s0, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST32_DTPREL_LO12, tls_ld_f
    fmov w2, s0
    ldr w3, =0x3f800000
    cmp w2, w3
    b.ne .Lfailure
    str s0, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST32_DTPREL_LO12, tls_ld_f
    ldr d0, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST64_DTPREL_LO12, tls_ld_g
    fmov x2, d0
    ldr x3, =0x3ff0000000000000
    cmp x2, x3
    b.ne .Lfailure
    str d0, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST64_DTPREL_LO12, tls_ld_g
    // Nonzero field addends and a negative addend within the same section.
    ldr x2, [x21]
    .reloc .-4, R_AARCH64_TLSLD_LDST64_DTPREL_LO12, tls_ld_q+8
    cmp x2, #22
    b.ne .Lfailure
    add x2, x21, #0
    .reloc .-4, R_AARCH64_TLSLD_ADD_DTPREL_LO12, tls_ld_h-2
    ldrb w2, [x2]
    cmp w2, #0x12
    b.ne .Lfailure
    bl tls_ld_check
    cmp w0, #1
    b.ne .Lfailure
    bl tls_read
    ldr w2, [x23]
    cmp w0, w2
    b.ne .Lfailure
    ldr w3, [x24]
    sub w4, w2, #7
    cmp w3, w4
    b.ne .Lfailure
    add w3, w3, #1
    str w3, [x24]
    add w3, w2, #1
    str w3, [x23]
    add w0, w2, w19
    add w0, w0, w20
    b .Ldone
.Lfailure:
    mov w0, #-1
.Ldone:
    ldp x23, x24, [sp, #48]
    ldp x21, x22, [sp, #32]
    ldp x19, x20, [sp, #16]
    ldp x29, x30, [sp], #80
    ret
.size tls_ld_step, .-tls_ld_step
.ltorg
.section .tdata,"awT",%progbits
.p2align 6
.type tls_ld_anchor, %tls_object
tls_ld_anchor:
.size tls_ld_anchor, 1
.p2align 0
.globl tls_ld_b
.hidden tls_ld_b
.type tls_ld_b, %tls_object
tls_ld_b:
    .byte 18
.size tls_ld_b, 1
.p2align 1
.globl tls_ld_h
.hidden tls_ld_h
.type tls_ld_h, %tls_object
tls_ld_h:
    .hword 4660
.size tls_ld_h, 2
.p2align 2
.globl tls_ld_w
.hidden tls_ld_w
.type tls_ld_w, %tls_object
tls_ld_w:
    .word 305419896
.size tls_ld_w, 4
.p2align 3
.globl tls_ld_d
.hidden tls_ld_d
.type tls_ld_d, %tls_object
tls_ld_d:
    .quad 1311768467463790320
.size tls_ld_d, 8
.p2align 4
.globl tls_ld_q
.hidden tls_ld_q
.type tls_ld_q, %tls_object
tls_ld_q:
    .quad 20, 22
.size tls_ld_q, 16
.p2align 2
.globl tls_ld_f
.hidden tls_ld_f
.type tls_ld_f, %tls_object
tls_ld_f:
    .word 1065353216
.size tls_ld_f, 4
.p2align 3
.globl tls_ld_g
.hidden tls_ld_g
.type tls_ld_g, %tls_object
tls_ld_g:
    .quad 4607182418800017408
.size tls_ld_g, 8
.section .tbss,"awT",%nobits
.p2align 6
.zero 65536
.type tls_ld_counter, %tls_object
tls_ld_counter:
.zero 4
.size tls_ld_counter, 4
.p2align 0
.globl tls_ld_zb
.hidden tls_ld_zb
.type tls_ld_zb, %tls_object
tls_ld_zb:
.zero 1
.size tls_ld_zb, 1
.p2align 1
.globl tls_ld_zh
.hidden tls_ld_zh
.type tls_ld_zh, %tls_object
tls_ld_zh:
.zero 2
.size tls_ld_zh, 2
.p2align 2
.globl tls_ld_zw
.hidden tls_ld_zw
.type tls_ld_zw, %tls_object
tls_ld_zw:
.zero 4
.size tls_ld_zw, 4
.p2align 3
.globl tls_ld_zd
.hidden tls_ld_zd
.type tls_ld_zd, %tls_object
tls_ld_zd:
.zero 8
.size tls_ld_zd, 8
.p2align 4
.globl tls_ld_zq
.hidden tls_ld_zq
.type tls_ld_zq, %tls_object
tls_ld_zq:
.zero 16
.size tls_ld_zq, 16
.section .note.GNU-stack,"",%progbits
