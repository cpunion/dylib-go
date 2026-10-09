.text
.globl got_abs_eval, got_based_eval, got_lea_eval, got_call_eval, got_jump_eval, weak_abs_eval
.macro got_base reg
    call .Lpc\@
.Lpc\@:
    popl \reg
    addl $_GLOBAL_OFFSET_TABLE_+(.-.Lpc\@), \reg
.endm
.macro finish
    movl 4(%edx), %eax
    addl 4(%esp), %eax
    addl 8(%esp), %eax
    ret
.endm
got_abs_eval:
    movl datum@GOT, %edx
    finish
got_based_eval:
    got_base %edx
    movl datum@GOT(%edx), %edx
    finish
got_lea_eval:
    // GNU's baseless LEA retains the GOT-relative offset.
    got_base %ecx
    leal datum@GOT, %edx
    addl %ecx, %edx
    movl (%edx), %edx
    finish
got_call_eval:
    // Forward both cdecl arguments and align the stack before the C call.
    subl $4, %esp
    pushl 12(%esp)
    pushl 12(%esp)
    call *via_table@GOT
    addl $12, %esp
    ret
got_jump_eval:
    jmp *via_table@GOT
weak_abs_eval:
    movl missing_got@GOT, %edx
    testl %edx, %edx
    jne .Lbadweak
    movl 4(%esp), %eax
    addl 8(%esp), %eax
    ret
.Lbadweak:
    movl $-1, %eax
    ret
.weak missing_got
.section .note.GNU-stack,"",@progbits
