#if defined(__x86_64__)
#define PC8(symbol,addend) .byte 0; .reloc .-1, R_X86_64_PC8, symbol+(addend)
#define PC16(symbol,addend) .short 0; .reloc .-2, R_X86_64_PC16, symbol+(addend)
#else
#define PC8(symbol,addend) .byte addend; .reloc .-1, R_386_PC8, symbol
#define PC16(symbol,addend) .short addend; .reloc .-2, R_386_PC16, symbol
#endif
.section .data,"aw",@progbits
.globl narrow_a8, narrow_a16, narrow_p8, narrow_m8, narrow_p16, narrow_m16, narrow_guard
.weak narrow_anchor8, narrow_anchor16
narrow_a8:
.byte narrow_value8 - 2
.balign 2
narrow_a16:
.short narrow_value16 - 2
narrow_anchor8:
narrow_p8:
PC8(narrow_anchor8,7)
narrow_m8:
PC8(narrow_anchor8,-8)
.balign 2
narrow_anchor16:
narrow_p16:
PC16(narrow_anchor16,11)
narrow_m16:
PC16(narrow_anchor16,-13)
narrow_guard:
.byte 0x5a
.section .note.GNU-stack,"",@progbits
