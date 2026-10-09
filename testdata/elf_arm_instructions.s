.text
.p2align 2
.globl address_eval, literal_eval, got_eval, cond_eval, zero_eval, nonzero_eval, bit_eval, bit_set_eval
address_eval:
    adr x2, datum+4
    ldr w2, [x2]
    add w0, w0, w1
    add w0, w0, w2
    ret
literal_eval:
    ldr w2, datum+4
    add w0, w0, w1
    add w0, w0, w2
    ret
got_eval:
    .reloc ., R_AARCH64_GOT_LD_PREL19, datum
    ldr x2, .
    ldr w2, [x2, #4]
    add w0, w0, w1
    add w0, w0, w2
    ret
cond_eval:
    cmp w0, w1
    b.lt target_less
    sub w0, w0, w1
    ret
zero_eval:
    cbz w0, target_zero
    add w0, w0, w1
    ret
nonzero_eval:
    cbnz w0, target_nonzero
    add w0, w0, w1
    ret
bit_eval:
    tbz w0, #0, target_even
    sub w0, w0, w1
    ret
bit_set_eval:
    tbnz w0, #0, target_odd
    sub w0, w0, w1
    ret
.section .note.GNU-stack,"",@progbits
