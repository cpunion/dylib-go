.text
.p2align 2
.globl unsigned_eval, signed_positive_eval, signed_negative_eval, signed_mid_eval, signed_high_eval
.globl prel_eval, prel_short_eval, prel_mid_eval, prel_high_eval, narrow_eval
unsigned_eval:
    movz x2, #:abs_g0:absolute_positive
    movz x3, #:abs_g1:absolute_positive
    movk x3, #:abs_g0_nc:absolute_positive
    movz x4, #:abs_g2:absolute_positive
    movk x4, #:abs_g1_nc:absolute_positive
    movk x4, #:abs_g0_nc:absolute_positive
    add w2, w2, w3
    add w2, w2, w4
    sub w2, w2, #12
    add w0, w0, w1
    add w0, w0, w2
    ret
signed_positive_eval:
    movn x2, #:abs_g0_s:absolute_positive
    add w2, w2, #24
    add w0, w0, w1
    add w0, w0, w2
    ret
signed_negative_eval:
    movz x2, #:abs_g0_s:absolute_negative
    add w2, w2, #66
    add w0, w0, w1
    add w0, w0, w2
    ret
signed_mid_eval:
    movz x2, #:abs_g1_s:absolute_negative
    movk x2, #:abs_g0_nc:absolute_negative
    add w2, w2, #66
    add w0, w0, w1
    add w0, w0, w2
    ret
signed_high_eval:
    movz x2, #:abs_g2_s:absolute_negative
    asr x2, x2, #32
    add w2, w2, #43
    add w0, w0, w1
    add w0, w0, w2
    ret
prel_eval:
    adr x2, .Lprel
.Lprel:
    // Each addend compensates for its different P, yielding datum+4-.Lprel.
    movz x3, #:prel_g3:datum+4
    movk x3, #:prel_g2_nc:datum+8
    movk x3, #:prel_g1_nc:datum+12
    movk x3, #:prel_g0_nc:datum+16
    add x2, x2, x3
    ldr w2, [x2]
    add w0, w0, w1
    add w0, w0, w2
    ret
prel_short_eval:
    adr x2, .Lshort
.Lshort:
    movz x3, #:prel_g0:datum+4
    add x2, x2, x3
    ldr w2, [x2]
    add w0, w0, w1
    add w0, w0, w2
    ret
prel_mid_eval:
    adr x2, .Lmid
.Lmid:
    movz x3, #:prel_g1:datum+4
    movk x3, #:prel_g0_nc:datum+8
    add x2, x2, x3
    ldr w2, [x2]
    add w0, w0, w1
    add w0, w0, w2
    ret
prel_high_eval:
    adr x2, .Lhigh
.Lhigh:
    movz x3, #:prel_g2:datum+4
    movk x3, #:prel_g1_nc:datum+8
    movk x3, #:prel_g0_nc:datum+12
    add x2, x2, x3
    ldr w2, [x2]
    add w0, w0, w1
    add w0, w0, w2
    ret
narrow_eval:
    adr x2, .Loffset
    ldrsh x3, [x2]
    add x2, x2, x3
    ldr w2, [x2]
    adr x3, .Lnegative16
    ldrsh w3, [x3]
    adr x4, .Lnegative32
    ldr w4, [x4]
    // Both ABS data widths must encode the negative SHN_ABS definition.
    sub w3, w3, w4
    add w0, w0, w1
    add w0, w0, w2
    add w0, w0, w3
    ret
.Loffset:
    .hword datum+4-.
.Lnegative16:
    .hword absolute_negative
.Lnegative32:
    .word absolute_negative
// A null relocation must not extract this unused dependency from an archive.
.reloc ., R_AARCH64_NONE, missing_dependency
.section .note.GNU-stack,"",@progbits
