# GNU ELF size-relocation fixtures

These small objects contain code/data written for this project, assembled with GNU as 2.42 from [elf_sizes.S](../elf_sizes.S). They are not copied from another library. The checked-in objects allow pure Go metadata tests on hosts without an x86 GNU assembler.

Regenerate on Linux x86 with `bash testdata/elfsizes/generate.sh`. Clang only preprocesses the assembly; GNU `as --64/--32` writes the objects. `TestELFSizeObjectMetadata` and `TestNativeELFSizeRelocations` regenerate temporary objects in Linux x86 CI, so runtime coverage does not rely solely on these snapshots.

Older LLVM assemblers rewrite local SIZE references to section references, including explicit `.reloc` directives. LLVM's i386 assembler does not implement `@SIZE`. GNU as retains the explicit local relocation and encodes the target's size correctly; these fixture choices preserve the original assertions across compiler versions.
