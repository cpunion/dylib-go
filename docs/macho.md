# Mach-O symbol aliases

Raw macOS amd64/arm64 objects support external `N_INDR` forwarding definitions. An alias owns its public name and resolves to its target's address. `n_value` is an offset into the object's string table, not a native address. The parser validates that offset, the terminating NUL, the nonempty target name, and the zero section ordinal. Names omit exactly one leading Mach-O linker underscore, including names containing dots.

Alias chains resolve through selected global definitions, common/absolute symbols, explicitly defined host targets, or session-owned libraries. An alias target is an external name; a same-named local symbol cannot satisfy it. Aliases without an existing target symbol receive a synthetic undefined external after the original symbol table, preserving relocation indices. Selected aliases introduce dependencies even when they have no relocations: an archive alias member pulls its target member, and providers initialize before their consumers. Unused archive members remain unselected.

Aliases are strong definitions, following Apple ld64's behavior. `N_WEAK_DEF` on the alias or a weak definition of its target does not weaken the alias's public name. Duplicate strong names fail; a host definition of the alias's name conflicts with the object definition. Host addresses can supply the target's name instead. Private-external `N_PEXT | N_EXT` aliases participate in linking and explicit `Lookup`, which does not implement Mach-O dylib export visibility. Non-external `N_INDR` records are ignored. COFF weak fallback and archive search rules remain separate.

Alias cycles and unresolved selected targets fail before image publication. Missing dependencies can be added before retrying `Link`. The tests compile genuine Mach-O symbols for both architectures, preserve alias chains that LLVM's assembler normally flattens, and execute object/archive calls with Go and llgo on macOS. They also check data relocations to alias addresses, private-external names, common/absolute/host targets, lazy extraction, malformed records, and retry.

This support does not implement every use of the Mach-O dynamic indirect symbol table. Indirect pointer/stub sections, general coalesced sections, authenticated arm64e symbols, raw TLS, and language runtime registration still require additional support. Complete `.dylib` images continue to use the OS loader.

## Format references

- [Apple ld64 Mach-O parser](https://github.com/apple-oss-distributions/ld64/blob/main/src/ld/parsers/macho_relocatable_file.cpp): `appendAliasAtoms` decodes external `N_INDR` records and their target names.
- [LLVM alias-symbol tests](https://github.com/llvm/llvm-project/blob/main/lld/test/MachO/alias-symbols.s): alias/private-external cases and the difference between ld64's strong alias definitions and LLD's inherited target strength.
- [LLVM Mach-O definitions](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/llvm/include/llvm/BinaryFormat/MachO.h): symbol flags and structures.
