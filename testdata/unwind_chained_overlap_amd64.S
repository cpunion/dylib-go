// LLVM's nested SEH directives emit three overlapping ranges for the leaf.
.text
.globl unwind_overlap_root
.def unwind_overlap_root; .scl 2; .type 32; .endef
.seh_proc unwind_overlap_root
unwind_overlap_root:
    subq $40, %rsp
    .seh_stackalloc 40
    .seh_endprologue
    callq unwind_overlap_leaf
    nop
    addq $40, %rsp
    retq
.seh_endproc

.p2align 4
.seh_proc unwind_overlap_leaf
unwind_overlap_leaf:
    subq $56, %rsp
    .seh_stackalloc 56
    .seh_endprologue
.Lprimary_body:
    nop
    .seh_startchained
.Lrbx_before:
    movq %rbx, 40(%rsp)
    .seh_savereg %rbx, 40
    .seh_endprologue
.Lrbx_body:
    nop
    .seh_startchained
    movq %rsi, 48(%rsp)
    .seh_savereg %rsi, 48
    .seh_endprologue
.Lrsi_body:
    nop
    callq capture_raw_frames
    nop // Do not let epilogue recognition bypass the chain.
    movq 48(%rsp), %rsi
    .seh_endchained
.Lresume_rbx:
    movq 40(%rsp), %rbx
    .seh_endchained
.Lresume_primary:
    nop // Exercise the resumed primary independently of its epilogue.
    addq $56, %rsp
    retq
.seh_endproc

.p2align 4
.globl unwind_overlap_probe
.def unwind_overlap_probe; .scl 2; .type 32; .endef
.seh_proc unwind_overlap_probe
unwind_overlap_probe:
    subq $40, %rsp
    .seh_stackalloc 40
    .seh_endprologue
    movl %ecx, %eax
    leaq .Lprobe_points(%rip), %rcx
    movq (%rcx,%rax,8), %rcx
    movl %eax, %edx
    callq verify_chain_context
    nop
    addq $40, %rsp
    retq
.seh_endproc

.section .rdata,"dr"
.p2align 3
.Lprobe_points:
    .quad unwind_overlap_leaf
    .quad .Lprimary_body
    .quad .Lrbx_before
    .quad .Lrbx_body
    .quad .Lrsi_body
    .quad .Lresume_rbx
    .quad .Lresume_primary
