#ifdef __x86_64__
#define SIZE32(symbol, addend) .reloc ., R_X86_64_SIZE32, symbol+addend; .long 0
#else
/* Assemble the i386 fixture with GNU as: LLVM does not implement @SIZE. */
#define SIZE32(symbol, addend) .long symbol@SIZE+addend
#endif

.data
.p2align 3
.globl size_value32, size_plus32, size_minus32, size_local32, size_common32, size_weak32
size_value32:
    SIZE32(payload, 0)
size_plus32:
    SIZE32(payload, 5)
size_minus32:
    SIZE32(payload, -5)
size_local32:
#ifdef __x86_64__
    SIZE32(local_payload, 0)
#else
    /* Keep a real local SIZE relocation instead of GNU as folding it. */
    .reloc ., R_386_SIZE32, local_payload
    .long 0
#endif
size_common32:
    SIZE32(common_payload, 0)
size_weak32:
    SIZE32(missing_weak, 0)
#ifdef __x86_64__
.globl size_value64
size_value64:
    .quad payload@SIZE+5
#endif

.local local_payload
.type local_payload,@object
local_payload:
    .zero 9
.size local_payload, .-local_payload
.comm common_payload,7,1
.weak missing_weak
.section .note.GNU-stack,"",@progbits
