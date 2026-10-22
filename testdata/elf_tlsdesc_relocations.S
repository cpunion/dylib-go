// Inspect GNU2 relocation records without requiring a recent C code generator.
// Native tests separately compile and execute Clang/GCC C and C++ GNU2 output.
.section .tdata,"awT",@progbits
.p2align 6
.globl tlsdesc_value
.type tlsdesc_value,@tls_object
tlsdesc_value: .long 7
.size tlsdesc_value,4
.section .tbss,"awT",@nobits
.p2align 6
.type tlsdesc_zero,@tls_object
tlsdesc_zero: .zero 64
.size tlsdesc_zero,64
.globl _TLS_MODULE_BASE_
.type _TLS_MODULE_BASE_,@tls_object
.text
.globl tlsdesc_addresses
.type tlsdesc_addresses,@function
tlsdesc_addresses:
#if defined(__x86_64__)
leaq tlsdesc_value@TLSDESC(%rip), %rax
call *tlsdesc_value@TLSCALL(%rax)
leaq tlsdesc_zero@TLSDESC(%rip), %rax
call *tlsdesc_zero@TLSCALL(%rax)
leaq _TLS_MODULE_BASE_@TLSDESC(%rip), %rax
call *_TLS_MODULE_BASE_@TLSCALL(%rax)
#else
leal tlsdesc_value@TLSDESC(%ebx), %eax
call *tlsdesc_value@TLSCALL(%eax)
leal tlsdesc_zero@TLSDESC(%ebx), %eax
call *tlsdesc_zero@TLSCALL(%eax)
leal _TLS_MODULE_BASE_@TLSDESC(%ebx), %eax
call *_TLS_MODULE_BASE_@TLSCALL(%eax)
#endif
ret
.size tlsdesc_addresses,.-tlsdesc_addresses
.section .note.GNU-stack,"",@progbits
