# ELF symbol-size relocations

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

The fixtures use Clang for amd64 and GNU `as --32` for i386. Explicit `.reloc` directives preserve local symbol references: LLVM can rewrite local `@SIZE` references into section references, while GNU as normally folds their known sizes. LLVM's i386 assembler does not implement `@SIZE`; it is not required to generate that fixture. Unit arithmetic tests run on all CI hosts, and actual execution remains limited to matching Linux x86 targets.

The independently written implementation follows the [GNU x86-64 relocation definitions](https://sourceware.org/git/?p=binutils-gdb.git;a=blob;f=bfd/elf64-x86-64.c) and [GNU i386 size relocation handling](https://sourceware.org/git/?p=binutils-gdb.git;a=blob;f=bfd/elf32-i386.c). It does not add dynamic-loader relocations, TLS, IFUNC, or new host architectures.
