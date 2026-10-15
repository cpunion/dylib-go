// A normal caller and a leaf split into three disjoint runtime-function ranges.
// The secondary ranges save registers without allocating another stack frame.
.text
.globl unwind_chain_root
.def unwind_chain_root; .scl 2; .type 32; .endef
.seh_proc unwind_chain_root
unwind_chain_root:
    subq $40, %rsp
    .seh_stackalloc 40
    .seh_endprologue
    callq unwind_chain_leaf
    nop // Keep the captured return PC outside the epilogue.
    addq $40, %rsp
    retq
.seh_endproc

.p2align 4
unwind_chain_leaf:
    subq $56, %rsp
.Lprimary_prolog_end:
    jmp .Lsave_rbx
.Lprimary_end:
.Lsave_rbx:
    movq %rbx, 40(%rsp)
.Lrbx_prolog_end:
    jmp .Lsave_rsi
.Lrbx_end:
.Lsave_rsi:
    movq %rsi, 48(%rsp)
.Lrsi_prolog_end:
    callq capture_raw_frames
    nop // Require the observer to follow both chain records.
    movq 48(%rsp), %rsi
    movq 40(%rsp), %rbx
    addq $56, %rsp
    retq
.Lleaf_end:

.section .xdata,"dr"
.p2align 2
.Lprimary_info:
    .byte 1, .Lprimary_prolog_end-unwind_chain_leaf, 1, 0
    .byte .Lprimary_prolog_end-unwind_chain_leaf, 0x62
    .short 0
.Lrbx_info:
    .byte 0x21, .Lrbx_prolog_end-.Lsave_rbx, 2, 0
    .byte .Lrbx_prolog_end-.Lsave_rbx, 0x34
    .short 5
    .rva unwind_chain_leaf, .Lprimary_end, .Lprimary_info
.Lrsi_info:
    .byte 0x21, .Lrsi_prolog_end-.Lsave_rsi, 2, 0
    .byte .Lrsi_prolog_end-.Lsave_rsi, 0x64
    .short 6
    .rva .Lsave_rbx, .Lrbx_end, .Lrbx_info

.section .pdata,"dr"
.p2align 2
    .rva unwind_chain_leaf, .Lprimary_end, .Lprimary_info
    .rva .Lsave_rbx, .Lrbx_end, .Lrbx_info
    .rva .Lsave_rsi, .Lleaf_end, .Lrsi_info
