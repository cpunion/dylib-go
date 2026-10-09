# ELF relocations

## Symbol-size relocations

`R_X86_64_SIZE32`, `R_X86_64_SIZE64`, and `R_386_SIZE32` write the selected symbol's size plus the relocation addend. They do not write a process address or a section size.

| Target | Relocation | Addend and result |
| --- | --- | --- |
| Linux amd64 | SIZE32 | Explicit RELA addend; result must fit unsigned 32 bits, matching GNU ld |
| Linux amd64 | SIZE64 | Explicit RELA addend; modulo-2^64 result |
| Linux 386 | SIZE32 | Implicit REL word or explicit RELA addend; modulo-2^32 result |

The implementation uses the existing definition selection: strong definitions override weak definitions, and local symbols retain object scope. A selected common symbol uses the maximum allocated size of its tentative definitions. Undefined weak symbols contribute zero when no external address resolves their name. The ELF null symbol also has size zero.

Strong undefined symbols fail. `Define` and OS-library symbol lookup supply addresses without ELF size metadata, so they cannot satisfy a size relocation. This also applies to a weak reference whose name resolves to an external address. Load a matching object definition with recorded size instead. Failure leaves the image unpublished; a missing object can be added before retrying `Link`.

Size references participate in ordinary archive dependency extraction even when there is no address relocation to their target. Unreferenced archive members remain unloaded. Writes outside the section, invalid symbol indexes, unloaded definitions, and amd64 SIZE32 overflow fail before publication.

`TestELFSizeRelocationArithmetic` covers unsigned bounds, negative addends, 64-bit and i386 wrapping, and REL/RELA behavior. `TestELFSizeObjectMetadata` inspects compiler/assembler-produced records. `TestNativeELFSizeRelocations` executes objects, weak overrides, archive dependencies/roots, retry, and a system-linker comparison on Linux amd64 and Go 386. Linux amd64 llgo runs the same tests through the existing native suite.

GNU `as --64/--32` generates the fixtures in native Linux x86 CI. Other hosts inspect checked-in snapshots with a [reproduction script](../testdata/elfsizes/generate.sh). Explicit `.reloc` directives preserve local symbol references: LLVM versions can rewrite even explicit local references into section references, while GNU as normally folds local `@SIZE` expressions. LLVM's i386 assembler does not implement `@SIZE`. Unit arithmetic and snapshot inspection run on all CI hosts; actual execution remains limited to matching Linux x86 targets.

The independently written implementation follows the [GNU x86-64 relocation definitions](https://sourceware.org/git/?p=binutils-gdb.git;a=blob;f=bfd/elf64-x86-64.c) and [GNU i386 size relocation handling](https://sourceware.org/git/?p=binutils-gdb.git;a=blob;f=bfd/elf32-i386.c). It does not add dynamic-loader relocations, TLS, IFUNC, or new host architectures.

## AArch64 instruction relocations

| Relocation | Instruction | Valid displacement |
| --- | --- | --- |
| ADR_PREL_LO21 | ADR | -1 MiB through +1 MiB minus one byte; byte offsets allowed |
| LD_PREL_LO19 | Integer/SIMD LDR, LDRSW, PRFM literal | -1 MiB through +1 MiB minus four bytes; multiple of four |
| CONDBR19 | B.cond, CBZ, CBNZ | Same signed, aligned 1 MiB range |
| TSTBR14 | TBZ, TBNZ | -32 KiB through +32 KiB minus four bytes; multiple of four |
| GOT_LD_PREL19 | 64-bit LDR literal from an image-owned pointer slot | Same signed, aligned 1 MiB range |

ELF RELA uses the explicit addend and replaces the encoded immediate. Local symbol scope, selected definitions, archives, GOT ownership, and publication rollback follow the ordinary linker rules. ADR and conditional branch encoders are shared with COFF, which first supplies its implicit instruction addend. These ADR, literal and conditional instructions must be aligned to four bytes and use the expected opcode. Literal and conditional references outside their ranges fail; the loader does not synthesize arbitrary branch islands or rewrite loads into different instruction sequences.

All AArch64 ELF relocations using `GDAT(S)` require zero addends, including the existing GOT page/load pair. The loader rejects nonzero addends before allocating the slot. Checked ADRP relocations retain their signed page range check; `ADR_PREL_PG_HI21_NC` truncates to the encoded field without a range diagnostic, as its non-checking contract requires. Unchecked encoding does not prove that the resulting instruction sequence addresses the caller's intended memory.

`TestARM64ELFInstructions` verifies opcodes, explicit-addend replacement, byte/scaled offsets, signed limits, register/bit-index preservation, bounds, and alignment. `TestARM64ELFGOTAndUncheckedPage` covers pointer slots, zero-addend validation, and checked/non-checking ADRP. `TestNativeARM64ELFInstructions` executes Clang-produced ADR/literal/GOT loads and both sides of B.cond/CBZ/CBNZ/TBZ/TBNZ branches on Linux arm64, including reversed objects, archive extraction/roots, unused members, and dependency retry. Both Go and llgo run these through the existing native CI suite.

The encoding and addend contracts follow Arm's [AAELF64 specification](https://github.com/ARM-software/abi-aa/blob/main/aaelf64/aaelf64.rst#static-aarch64-relocations). Other AArch64 relocation families, TLS, and authenticated pointers remain outside this extension.
