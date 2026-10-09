#ifndef EVAL_TARGET
#define EVAL_TARGET _eval
#endif
#if defined(__aarch64__)
.section __TEXT,__stubs,symbol_stubs,pure_instructions,12
.p2align 2
.globl _stub_eval
_stub_eval:
.indirect_symbol EVAL_TARGET
adrp x16,_lazy_eval@PAGE
ldr x16,[x16,_lazy_eval@PAGEOFF]
br x16
#else
.section __TEXT,__stubs,symbol_stubs,pure_instructions,6
.globl _stub_eval
_stub_eval:
.indirect_symbol EVAL_TARGET
jmpq *_lazy_eval(%rip)
#endif

.section __DATA,__la_symbol_ptr,lazy_symbol_pointers
.p2align 3
.globl _lazy_eval
_lazy_eval:
.indirect_symbol EVAL_TARGET
.quad 0x1234

.section __DATA,__nl_symbol_ptr,non_lazy_symbol_pointers
.p2align 3
.globl _nonlazy_eval
_nonlazy_eval:
.indirect_symbol EVAL_TARGET
.quad 0x5678
.globl _local_pointer
_local_pointer:
.indirect_symbol _local_number
.quad _local_number

.section __DATA,__data
.p2align 2
_local_number:
.long 42
.subsections_via_symbols
