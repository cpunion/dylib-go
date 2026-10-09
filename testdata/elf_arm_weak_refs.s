.weak weak_value
.data
.globl weak_offsets
weak_offsets:
    .hword weak_value+42-.
    .word weak_value+43-.
    .quad weak_value+44-.
    .quad weak_value
.section .note.GNU-stack,"",@progbits
