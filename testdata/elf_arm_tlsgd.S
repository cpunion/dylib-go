// Traditional AArch64 GD indices, independent of a C driver's TLS dialect.
.text
.globl tls_gd_address
.type tls_gd_address, %function
tls_gd_address:
stp x29, x30, [sp, #-16]!
mov x29, sp
adrp x0, 0
.reloc .-4, R_AARCH64_TLSGD_ADR_PAGE21, tls_initialized
add x0, x0, #0
.reloc .-4, R_AARCH64_TLSGD_ADD_LO12_NC, tls_initialized
bl __tls_get_addr
adrp x0, 0
.reloc .-4, R_AARCH64_TLSGD_ADR_PAGE21, tls_zero
add x0, x0, #0
.reloc .-4, R_AARCH64_TLSGD_ADD_LO12_NC, tls_zero
bl __tls_get_addr
ldp x29, x30, [sp], #16
ret
.size tls_gd_address, .-tls_gd_address

.section .tdata,"awT",%progbits
.p2align 6
.globl tls_initialized
.type tls_initialized, %tls_object
tls_initialized:
.word 7
.size tls_initialized, 4
.section .tbss,"awT",%nobits
.p2align 6
.type tls_zero, %tls_object
tls_zero:
.zero 64
.size tls_zero, 64
.section .note.GNU-stack,"",%progbits
