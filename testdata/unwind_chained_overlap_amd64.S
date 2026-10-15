// LLVM emits overlapping .pdata ranges for nested chained directives.
.text
.seh_proc overlapping_chain
overlapping_chain:
    subq $40, %rsp
    .seh_stackalloc 40
    .seh_endprologue
    .seh_startchained
    .seh_endprologue
    nop
    .seh_endchained
    addq $40, %rsp
    retq
.seh_endproc
