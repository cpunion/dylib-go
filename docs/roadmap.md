# Implemented scope and future work

The current implementation is a standalone Go native object loader with real call tests: content identification, a shared object model, archive extraction, local/global/weak/common symbols, an explicit relocation subset, adjacent GOT/branch stubs, W^X memory, rollback, generic symbol lifetime guards, OS libraries, caller-defined gc/llgo adapters, optional dynamic scalar/struct signatures, and leased C callback entries for Go handlers.

The CLI now accepts Go-style signature declarations and typed invocations through the independent `abi/signature` package. It supports fixed-width scalars, pointers, ordinary C structs, and fixed-length arrays inside structs or behind pointers. These ABI improvements do not broaden object relocation or language runtime coverage.

## What complete coverage would require

"Complete" needs a bounded target, object format, ABI, toolchain, and runtime feature set. More parser cases or signature spellings cannot establish arbitrary native execution.

| Coverage area | Work still required |
| --- | --- |
| Current 64-bit raw targets | Remaining relocations, Mach-O coalescing/indirect symbols, remaining lifecycle forms, TLS, and remaining POSIX/Mach-O unwind registration; Windows/Linux/macOS C frame tables are optional |
| Windows ARM64 raw objects | Nonstandard import tables, remaining relocations/lifecycle forms, and language exception handlers; C frame tables are optional |
| 386 and additional ISAs | 386 executes C cdecl on Linux/Windows with Go; Windows 386 stdcall/fastcall are implemented; other conventions and llgo qualification remain. Additional ISAs need linker/ABI backends and execution tests |
| BSD and Apple mobile hosts | Native OS backend, ABI/runtime integration, platform policy compatibility, and device tests |
| Additional containers | Nonstandard import tables; optional IR compilation |
| Full dynamic C/C++ interfaces | Unions, packed structs, bitfields, additional callback conventions/aggregates, and generated declaration validation |
| Swift, ObjC, and other language ABIs | Dedicated metadata, ownership, calling-convention, initialization, and runtime adapters |
| Foreign CPU or OS execution | An appropriate emulator/system process plus RPC; native relocation cannot supply OS services |
| Wasm, GPU, eBPF | Separate executors, drivers, or kernel loaders, with independent interfaces and tests |

Complete shared libraries already use the host OS loader for their dependency, initialization, TLS, and native unwind metadata. Calls still require correct ABI adapters, language runtime contracts, and lifetimes. There is no current claim that every language feature works merely because its image loads.

## Implementation priorities

Future extensions need explicit implementation and tests:

1. **Windows object coverage:** native Go and llgo objects, archives, and DLLs are covered for AMD64/ARM64, plus Go for i386. Weak aliases, COMDAT selections 1–7, bigobj, extended relocation tables, short/GNU long import libraries, and optional C frame unwind registration are implemented. Further work includes nonstandard import tables, remaining lifecycle forms, language exception handlers, and additional architecture-specific relocations.
2. **Object initialization and exceptions:** modern platform tables, legacy ELF `.ctors/.dtors`, COFF integer-returning initialization/failure cleanup, dependency ordering, cycle handling, validation rollback, session-owned exit registration, and optional Windows/Linux/macOS C frame tables are implemented. Add ELF executable startup fragments and Mach-O relative offsets; extend the implemented libgcc `.eh_frame` subset, extend Mach-O DWARF and add compact unwind, and extend Windows unwind records. Complete OS libraries already use their native runtime registration.
3. **TLS and more relocations:** define each target's TLS model and thread registration/destruction; extend ARM64 COFF coverage and add RISC-V, LoongArch, and other relocation backends. Parser support alone does not provide execution support.
4. **Archive and object containers:** GNU/LLVM thin archives, GNU member-offset references, and universal Mach-O object/archive/library selection are implemented. Add optional LLVM bitcode/IR compilation. OMF and D `.ddl` compatibility remain outside scope.
5. **llcppg dynamic generation:** complement its static `go:linkname` output with adapters using `Resolve` and `WithAddress`, preserving declaration-based ABI validation.
6. **ABIBridge adapters:** expose Apple Swift/ObjC values and calls through opaque C ABI handles. Platform, runtime version, and pointer-authentication coverage need separate tests.
7. **Advanced signatures and callbacks:** extend the implemented libffi structs, fixed-length array members/pointees, and callbacks with unions and other aggregate forms; evaluate compiler-generated llgo entries for known signatures. Fixed native C callbacks, leases/retirement, Go captures, foreign-thread integration, variadic call promotion, Windows 386 stdcall/fastcall, owned reusable CIFs/layouts, session-owned exact-signature caches, lazy typed pointee layouts, and call/allocation benchmarks are implemented; ABI-equivalent normalization, indexed cache lookup, and buffer pooling remain pending. Known signatures can already use caller-defined adapters.
8. **Independent execution backends:** isolated/remote processes, Wasm, and GPU execution need separate interfaces appropriate to their execution models.

Keep the parser and linker in Go and extend llgo or small native adapters when lower-level facilities are required. A future optional LLVM backend should preserve the independent Go backend rather than make LLVM a default dependency.

## Development checklist

Each implementation PR must pass its native Go and llgo CI jobs before the next item starts. A checked item describes implemented behavior with tests, not universal toolchain compatibility.

- [x] Fixed-length array members and pointees, including nested arrays/records and callbacks.
- [x] COFF weak fallback chains, archive search policies, and COMDAT EXACT_MATCH/NEWEST.
- [x] COFF bigobj and extended relocation tables, including high section indexes and real object/archive calls.
- [x] COFF short import libraries, with DLL dependency and real call tests.
- [x] Standard GNU COFF long import libraries, including MinGW consumer and genuine GNU dlltool fixture execution.
- [ ] Mixed/custom COFF import layouts and delay import tables.
- [ ] Remaining relocations on existing targets; Mach-O indirect symbols/coalesced sections.
- [x] Legacy ELF `.ctors/.dtors` pointer tables, priorities, sentinels, and mixed modern-array execution.
- [x] COFF integer-returning CRT initializers, permanent failure state, and owned exit-registration cleanup.
- [ ] ELF `.init/.fini` executable startup fragments and Mach-O initializer offsets.
- [x] Optional Windows amd64/arm64 C runtime function tables, OS stack traversal, and owned unregistration.
- [x] Optional Linux amd64/arm64/386 C `.eh_frame` registration with retained libgcc, native stack traversal and deregistration.
- [x] Optional macOS amd64/arm64 C DWARF registration with system libunwind, implicit FDE rebasing, native traversal and deregistration.
- [ ] Windows language/chained unwind records, POSIX personality/LSDA and additional providers, and Mach-O compact unwind registration.
- [x] Concurrent Symbol/Function/CallPlan calls and reentrant callbacks, with retirement before cleanup and no invocation lock across native calls.
- [ ] Reentrant load/link initializers that can resolve staged symbols before publication.
- [ ] Explicit raw-object TLS models, per-thread allocation, and thread destructor ownership.
- [ ] Union, packed record, and bitfield layouts through validated compiler adapters where libffi cannot represent the ABI.
- [x] Dynamic argument counts for scalar/aggregate calls, variadic shapes, and callbacks; reproducible call/allocation benchmarks.
- [x] Session-owned exact-signature CIF reuse, aggregate layout deduplication, and lazy typed pointee layout caches with rollback.
- [ ] ABI-equivalent cache normalization, indexed lookup, and independent invocation buffer pooling.
- [x] GNU/LLVM thin archives, relative paths, regular-member proxies and object snapshots.
- [x] Universal Mach-O FAT32/FAT64 inspection and baseline macOS object/archive/library selection.
- [ ] Clang/llcppg declaration-based dynamic adapter generation.
- [ ] C++ constructor/method/object adapters, followed by inheritance and virtual dispatch.
- [ ] Public providers, dependency paths/manifests, and version contracts.
- [ ] Owned native values and long-lived native registrations.
- [ ] Module replacement with explicit retirement and state migration.
- [ ] Optional Apple C facades for ABIBridge Swift/ObjC integration.
- [ ] BSD backends, additional ISAs, and llgo 386 qualification when toolchains/runners are available.
- [ ] Optional LLVM IR/bitcode compiler provider.
- [ ] Separate process/RPC and Wasm executors; GPU/eBPF require domain-specific executors.
