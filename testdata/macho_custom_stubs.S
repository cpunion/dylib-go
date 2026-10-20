#ifndef STUB_WIDTH
#error STUB_WIDTH is required
#endif

.section __TEXT,__stubs,symbol_stubs,pure_instructions,STUB_WIDTH
#if defined(__aarch64__)
.p2align 2
#endif
.globl _stub_eval
_stub_eval:
.indirect_symbol _eval
.space STUB_WIDTH,0xcc

.globl _stub_other
_stub_other:
.indirect_symbol _other
.space STUB_WIDTH,0xcc
.subsections_via_symbols
