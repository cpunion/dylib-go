.text
// Allocate a distinct slot before the datum offsets are used.
movq seed_value@GOTPCREL(%rip), %rax

.macro got_base
    leaq _GLOBAL_OFFSET_TABLE_(%rip), %rdx
.endm
.macro finish
    movl 4(%rdx), %eax
    addl %edi, %eax
    addl %esi, %eax
    ret
.endm

.globl got32_eval, got64_eval, gotoff64_eval, gotpcrel64_eval
got32_eval:
    got_base
    movslq .Lgot32(%rip), %rcx
    subq $4, %rcx
    movq (%rdx,%rcx), %rdx
    finish
.Lgot32:
    .reloc ., R_X86_64_GOT32, datum+4
    .long 0
got64_eval:
    got_base
    movq .Lgot64(%rip), %rcx
    subq $4, %rcx
    movq (%rdx,%rcx), %rdx
    finish
.Lgot64:
    .reloc ., R_X86_64_GOT64, datum+4
    .quad 0
gotoff64_eval:
    got_base
    movabsq $datum@GOTOFF, %rcx
    addq %rcx, %rdx
    finish
gotpcrel64_eval:
    leaq .Lgotpcrel64-4(%rip), %rdx
    addq .Lgotpcrel64(%rip), %rdx
    movq (%rdx), %rdx
    finish
.Lgotpcrel64:
    .reloc ., R_X86_64_GOTPCREL64, datum+4
    .quad 0

.globl gotpc64_eval, gotplt64_eval, pltoff64_eval, weak_got_eval
gotpc64_eval:
    leaq .Lgotpc64-4(%rip), %rdx
    addq .Lgotpc64(%rip), %rdx
    movabsq $datum@GOT, %rcx
    movq (%rdx,%rcx), %rdx
    finish
.Lgotpc64:
    .reloc ., R_X86_64_GOTPC64, _GLOBAL_OFFSET_TABLE_+4
    .quad 0
gotplt64_eval:
    got_base
    movq .Lgotplt64(%rip), %rcx
    subq $4, %rcx
    jmp *(%rdx,%rcx)
.Lgotplt64:
    .reloc ., R_X86_64_GOTPLT64, via_table+4
    .quad 0
pltoff64_eval:
    got_base
    movabsq $via_table@PLTOFF, %rcx
    addq %rcx, %rdx
    jmp *%rdx
weak_got_eval:
    got_base
    movabsq $missing_got@GOT, %rcx
    cmpq $0, (%rdx,%rcx)
    jne .Lbadweak
    leal (%rdi,%rsi), %eax
    ret
.Lbadweak:
    movl $-1, %eax
    ret
.weak missing_got

.data
// These fields have no symbol operand. Neither the null symbol nor a named
// undefined symbol should be resolved or cause archive extraction.
.globl base_null, base_ignored
base_null:
    .reloc ., R_X86_64_GOTPC32, 0
    .long 0
base_ignored:
    .reloc ., R_X86_64_GOTPC64, ignored_got_symbol+4
    .quad 0
.section .note.GNU-stack,"",@progbits
