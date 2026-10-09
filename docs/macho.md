# Mach-O aliases, pointers, and stubs

## Forwarding aliases

Raw macOS amd64/arm64 objects support external `N_INDR` forwarding definitions. An alias owns its public name and resolves to its target's address. `n_value` is an offset into the object's string table, not a native address. The parser validates that offset, the terminating NUL, the nonempty target name, and the zero section ordinal. Names omit exactly one leading Mach-O linker underscore, including names containing dots.

Alias chains resolve through selected global definitions, common/absolute symbols, explicitly defined host targets, or session-owned libraries. An alias target is an external name; a same-named local symbol cannot satisfy it. Aliases without an existing target symbol receive a synthetic undefined external after the original symbol table, preserving relocation indices. Selected aliases introduce dependencies even when they have no relocations: an archive alias member pulls its target member, and providers initialize before their consumers. Unused archive members remain unselected.

Aliases are strong definitions, following Apple ld64's behavior. `N_WEAK_DEF` on the alias or a weak definition of its target does not weaken the alias's public name. Duplicate strong names fail; a host definition of the alias's name conflicts with the object definition. Host addresses can supply the target's name instead. Private-external `N_PEXT | N_EXT` aliases participate in linking and explicit `Lookup`, which does not implement Mach-O dylib export visibility. Non-external `N_INDR` records are ignored. COFF weak fallback and archive search rules remain separate.

Alias cycles and unresolved selected targets fail before image publication. Missing dependencies can be added before retrying `Link`. The tests compile genuine Mach-O symbols for both architectures, preserve alias chains that LLVM's assembler normally flattens, and execute object/archive calls with Go and llgo on macOS. They also check data relocations to alias addresses, private-external names, common/absolute/host targets, lazy extraction, malformed records, and retry.

## Indirect pointer and stub tables

The Go parser reads `reserved1`/`reserved2` from raw section headers and validates their ranges in `LC_DYSYMTAB`'s indirect symbol table. Original nlist indices remain intact when forwarding aliases add synthetic targets. Table entries become explicit fixups before dependency discovery, so their targets participate in archive extraction and initializer ordering.

| Section type | Raw-object behavior |
| --- | --- |
| `S_NON_LAZY_SYMBOL_POINTERS` | Resolve each symbol and write its address before execution |
| `S_LAZY_SYMBOL_POINTERS` | Eagerly replace the initial resolver/helper word with the target address |
| `S_LAZY_DYLIB_SYMBOL_POINTERS` | Same eager binding, using already loaded session libraries or host definitions |
| `S_SYMBOL_STUBS`, amd64 width 6 | Regenerate `jmp [rip + displacement]` through an image-owned GOT slot |
| `S_SYMBOL_STUBS`, arm64 width 12 | Regenerate `adrp x16; ldr x16; br x16` through an image-owned GOT slot |

Pointer binding overwrites placeholder words rather than adding them to the resolved address. Original pointer fixups are retained only for local markers; global table entries and standard stubs replace their old fixups. Stub regeneration preserves parameter registers, stack arguments, floating-point registers, and the caller's return address. ARM64 uses the ABI's interprocedural scratch register `x16`, leaving the hidden result pointer in `x8` intact. Binding is complete before W^X protection and publication; no dyld lazy resolver is installed.

Non-lazy `INDIRECT_SYMBOL_LOCAL` pointers use an existing pointer relocation or rebase their stored original address to an unambiguous retained section of the same object. `INDIRECT_SYMBOL_LOCAL | INDIRECT_SYMBOL_ABS` retains a numeric address without relocation. Missing targets, invalid symbol indices/ranges, partial entries, zero stub widths, misplaced markers, and invalid local addresses fail before execution. Unimplemented stub widths remain inspectable but block raw linking. Failure before initialization allows adding dependencies or host target definitions and retrying.

Tests compile both target formats on every CI host. Native macOS Go and llgo tests call pointers and stubs from objects/archives, through forwarding aliases and host targets, and with stripped local fixups and lazy-dylib pointer types. A 24-byte C struct parameter/result with a floating-point argument exercises stack/register arguments and hidden aggregate-result state.

General coalesced sections, other custom stub layouts, authenticated arm64e symbols, raw TLS, and language runtime registration still require additional support. Complete `.dylib` images continue to use the OS loader.

## Format references

- [Apple ld64 Mach-O parser](https://github.com/apple-oss-distributions/ld64/blob/main/src/ld/parsers/macho_relocatable_file.cpp): `appendAliasAtoms` decodes external `N_INDR` records and their target names.
- [LLVM alias-symbol tests](https://github.com/llvm/llvm-project/blob/main/lld/test/MachO/alias-symbols.s): alias/private-external cases and the difference between ld64's strong alias definitions and LLD's inherited target strength.
- [LLVM Mach-O definitions](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/llvm/include/llvm/BinaryFormat/MachO.h): symbol flags and structures.
- [Apple Mach-O headers](https://github.com/apple-oss-distributions/xnu/blob/main/EXTERNAL_HEADERS/mach-o/loader.h): section types, indirect-table indexing, and local/absolute markers.
- [Arm procedure call standard](https://github.com/ARM-software/abi-aa/blob/main/aapcs64/aapcs64.rst): interprocedural scratch registers and the indirect result location register.
