// Descriptor address construction shared by value and register probes.
#ifndef DYLIB_TLSDESC_FORM
#define DYLIB_TLSDESC_FORM 3
#endif
.macro tls_desc_pointer symbol
#if DYLIB_TLSDESC_FORM == 0
    ldr x1, .
    .reloc .-4, R_AARCH64_TLSDESC_LD_PREL19, \symbol
    adr x0, .
    .reloc .-4, R_AARCH64_TLSDESC_ADR_PREL21, \symbol
#elif DYLIB_TLSDESC_FORM == 1
    adr x0, .
    .reloc .-4, R_AARCH64_TLSDESC_ADR_PREL21, \symbol
    ldr x1, [x0]
    .reloc .-4, R_AARCH64_TLSDESC_LDR, \symbol
    add x0, x0, #0
    .reloc .-4, R_AARCH64_TLSDESC_ADD, \symbol
#elif DYLIB_TLSDESC_FORM == 2
    adrp x2, _GLOBAL_OFFSET_TABLE_
    add x2, x2, :lo12:_GLOBAL_OFFSET_TABLE_
    movz x0, #0, lsl #16
    .reloc .-4, R_AARCH64_TLSDESC_OFF_G1, \symbol
    movk x0, #0
    .reloc .-4, R_AARCH64_TLSDESC_OFF_G0_NC, \symbol
    ldr x1, [x2, x0]
    .reloc .-4, R_AARCH64_TLSDESC_LDR, \symbol
    add x0, x2, x0
    .reloc .-4, R_AARCH64_TLSDESC_ADD, \symbol
#else
    adrp x0, :tlsdesc:\symbol
    ldr x1, [x0, :tlsdesc_lo12:\symbol]
    add x0, x0, :tlsdesc_lo12:\symbol
#endif
.endm
