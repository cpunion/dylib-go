.text
.p2align 2
// Seed a different slot so the exercised datum offset is nonzero.
ldr x9, :got:seed_value

.macro got_base reg
    adrp \reg, _GLOBAL_OFFSET_TABLE_
    add \reg, \reg, :lo12:_GLOBAL_OFFSET_TABLE_
.endm
.macro finish
    ldr w2, [x2, #4]
    add w0, w0, w1
    add w0, w0, w2
    ret
.endm

.globl gotpage_eval, gotoff_eval
gotpage_eval:
    adrp x2, _GLOBAL_OFFSET_TABLE_
    ldr x2, [x2, :gotpage_lo15:datum]
    finish
gotoff_eval:
    got_base x2
    .reloc ., R_AARCH64_LD64_GOTOFF_LO15, datum
    ldr x2, [x2]
    finish

.macro wide_eval name, group
.globl \name
\name:
    got_base x2
.if \group == 3
    .reloc ., R_AARCH64_MOVW_GOTOFF_G3, datum
    movz x3, #0, lsl #48
    .reloc ., R_AARCH64_MOVW_GOTOFF_G2_NC, datum
    movk x3, #0, lsl #32
.elseif \group == 2
    .reloc ., R_AARCH64_MOVW_GOTOFF_G2, datum
    movz x3, #0, lsl #32
.endif
.if \group >= 2
    .reloc ., R_AARCH64_MOVW_GOTOFF_G1_NC, datum
    movk x3, #0, lsl #16
.elseif \group == 1
    .reloc ., R_AARCH64_MOVW_GOTOFF_G1, datum
    movz x3, #0, lsl #16
.endif
.if \group >= 1
    .reloc ., R_AARCH64_MOVW_GOTOFF_G0_NC, datum
    movk x3, #0
.else
    .reloc ., R_AARCH64_MOVW_GOTOFF_G0, datum
    movz x3, #0
.endif
    ldr x2, [x2, x3]
    finish
.endm
wide_eval gotoff_g0_eval, 0
wide_eval gotoff_g1_eval, 1
wide_eval gotoff_g2_eval, 2
wide_eval gotoff_g3_eval, 3

.globl gotrel64_eval, gotrel32_eval, gotpcrel_eval, plt32_eval, weak_got_eval
gotrel64_eval:
    got_base x2
    ldr x3, .Lgotrel64
    add x2, x2, x3
    ldr w2, [x2]
    add w0, w0, w1
    add w0, w0, w2
    ret
.p2align 3
.Lgotrel64:
    .reloc ., R_AARCH64_GOTREL64, datum+4
    .quad 0
gotrel32_eval:
    got_base x2
    ldrsw x3, .Lgotrel32
    add x2, x2, x3
    ldr w2, [x2]
    add w0, w0, w1
    add w0, w0, w2
    ret
.Lgotrel32:
    .reloc ., R_AARCH64_GOTREL32, datum+4
    .word 0
gotpcrel_eval:
    // The field bias changes the decoding anchor, never the GOT target.
    adr x2, .Lgotpc-4
    ldrsw x3, [x2, #4]
    add x2, x2, x3
    ldr x2, [x2]
    finish
.Lgotpc:
    .word datum@GOTPCREL+4
plt32_eval:
    adr x2, .Lplt-4
    ldrsw x3, [x2, #4]
    add x2, x2, x3
    br x2
.Lplt:
    .word via_table@plt-.+4
.weak missing_got
weak_got_eval:
    adr x2, .Lweakgot-4
    ldrsw x3, [x2, #4]
    add x2, x2, x3
    ldr x2, [x2]
    cbz x2, .Lweakdone
    ldr w2, [x2, #4]
    add w0, w0, w2
.Lweakdone:
    add w0, w0, w1
    ret
.Lweakgot:
    .word missing_got@GOTPCREL+4
.section .note.GNU-stack,"",@progbits
