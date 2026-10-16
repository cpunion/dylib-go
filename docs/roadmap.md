# Implemented scope and future work

The current implementation is a standalone Go native object loader with real call tests: content identification, a shared object model, archive extraction, local/global/weak/common symbols, an explicit relocation subset, adjacent GOT/branch stubs, W^X memory, rollback, generic symbol lifetime guards, OS libraries, caller-defined gc/llgo adapters, optional dynamic scalar/struct signatures, and leased C callback entries for Go handlers.

The CLI now accepts Go-style signature declarations and typed invocations through the independent `abi/signature` package. It supports fixed-width scalars, pointers, ordinary C structs, and fixed-length arrays inside structs or behind pointers. These ABI improvements do not broaden object relocation or language runtime coverage.

## What complete coverage would require

"Complete" needs a bounded target, object format, ABI, toolchain, and runtime feature set. More parser cases or signature spellings cannot establish arbitrary native execution.

| Coverage area | Work still required |
| --- | --- |
| Current 64-bit raw targets | Remaining relocations, Mach-O general coalescing/custom stubs, remaining lifecycle forms, TLS, and remaining POSIX/Mach-O unwind registration; external `N_INDR` aliases, eager indirect pointers/standard stubs, and optional Windows/Linux/macOS C frame tables plus the Linux C++ DWARF and macOS C++ DWARF/compact exception subsets are implemented |
| Windows ARM64 raw objects | Nonstandard import tables, remaining relocations/lifecycle forms, and language exception handlers; C frame tables are optional |
| 386 and additional ISAs | 386 executes C cdecl on Linux/Windows with Go; Windows 386 stdcall/fastcall are implemented; other conventions and llgo qualification remain. Additional ISAs need linker/ABI backends and execution tests |
| BSD and Apple mobile hosts | Native OS backend, ABI/runtime integration, platform policy compatibility, and device tests |
| Additional containers | Nonstandard import tables and automatic LTO archive member selection |
| Full dynamic C/C++ interfaces | Unions, remaining packing/alignment forms, callback conventions/aggregates, and C++ adapters; explicit packed/aligned structs and bitfields use compiled C bridges |
| Swift, ObjC, and other language ABIs | Dedicated metadata, ownership, calling-convention, initialization, and runtime adapters |
| Foreign CPU or OS execution | An appropriate emulator/system process plus RPC; native relocation cannot supply OS services |
| Wasm, GPU, eBPF | Separate executors, drivers, or kernel loaders, with independent interfaces and tests |

Complete shared libraries already use the host OS loader for their dependency, initialization, TLS, and native unwind metadata. Calls still require correct ABI adapters, language runtime contracts, and lifetimes. There is no current claim that every language feature works merely because its image loads.

## Implementation priorities

Future extensions need explicit implementation and tests:

1. **Windows object coverage:** native Go and llgo objects, archives, and DLLs are covered for AMD64/ARM64, plus Go for i386. Weak aliases, COMDAT selections 1–7, bigobj, extended relocation tables, short/GNU long import libraries, and optional C frame unwind registration are implemented. Further work includes nonstandard import tables, remaining lifecycle forms, language exception handlers, and additional architecture-specific relocations.
2. **Object initialization and exceptions:** modern platform tables, legacy ELF `.ctors/.dtors`, Mach-O initializer offsets, COFF integer-returning initialization/failure cleanup, GNU ELF split CRT startup fragments, dependency ordering, cycle handling, validation rollback, session-owned exit registration, and optional Windows/Linux/macOS C frame tables plus the Linux C++ DWARF and macOS C++ DWARF/compact exception subsets are implemented. Extend the supported split CRT frame forms; extend the implemented libgcc `.eh_frame` subset, extend Mach-O DWARF/compact unwind, and extend Windows unwind records. Complete OS libraries already use their native runtime registration.
3. **TLS and more relocations:** define each target's TLS model and thread registration/destruction; extend ARM64 COFF coverage and add RISC-V, LoongArch, and other relocation backends. Parser support alone does not provide execution support.
4. **Archive and object containers:** GNU/LLVM thin archives, GNU member-offset references, universal Mach-O object/archive/library selection, and explicit optional LLVM bitcode/IR compilation are implemented. Ordinary/thin/proxy bitcode/native mixed archive compilation, source-directory metadata and explicit module merging are implemented. OMF and D `.ddl` compatibility remain outside scope.
5. **llcppg dynamic generation:** optional Clang C17 scalar/typedef/record/pointer declarations, supported calling conventions, variadic prefixes and target-qualified Go source are implemented for `Session.Bind`. Native ABI size/alignment/offset queries and compiler/backend record layout validation are implemented. Function-boundary pointer signatures, generated struct callbacks and native pointer factories are implemented. Direct fixed-scalar/record/native-pointer cdecl llgo adapters with compiler layout checks are implemented without libffi. Fixed native function-pointer arguments/results and same-thread noncapturing C entries are implemented. Static cgo bridges for Go/llgo provide typed scalar/native-address calls and concrete variadic shapes without libffi. Direct llgo variadic methods and native variadic pointers are implemented with C promotions. Explicit C++17 class manifests generate scalar/native-address C facades with constructors, members, native virtual dispatch and exception status codes. Explicit class values/references/pointers and exact native function pointers with retained Go callbacks are implemented; named lambda/`std::function` call operators, copies and move-only captures are implemented. Extend declaration discovery, complementing llcppg's static output.
6. **ABIBridge adapters:** native Swift/ObjC++ C facades with explicit opaque object references, leased Go observers and destruction during owner retirement are qualified on macOS. ABIBridge-backed values/calls, runtime versions, UI threading and pointer authentication need separate implementation and tests.
7. **Advanced signatures and callbacks:** extend the implemented libffi structs, fixed-length array members/pointees, and callbacks with unions and other aggregate forms; evaluate compiler-generated llgo entries for known signatures. Fixed native C callbacks, leases/retirement, Go captures, foreign-thread integration, variadic call promotion, Windows 386 stdcall/fastcall, owned reusable CIFs/layouts, indexed session-owned logical signature caches, lazy typed pointee layouts, bounded scalar/aggregate/pointee buffer reuse, and call/allocation benchmarks are implemented; conservative sharing of physical ABI resources between logically distinct signatures is implemented. Known signatures can already use caller-defined adapters.
8. **Independent execution backends:** isolated/remote processes, Wasm, and GPU execution need separate interfaces appropriate to their execution models.

Keep the parser and linker in Go and extend llgo or small native adapters when lower-level facilities are required. Optional LLVM tools preserve the independent Go backend and do not make LLVM a default dependency.

## Development checklist

Each implementation PR must pass its native Go and llgo CI jobs before the next item starts. A checked item describes implemented behavior with tests, not universal toolchain compatibility.

- [x] Fixed-length array members and pointees, including nested arrays/records and callbacks.
- [x] COFF weak fallback chains, archive search policies, and COMDAT EXACT_MATCH/NEWEST.
- [x] COFF bigobj and extended relocation tables, including high section indexes and real object/archive calls.
- [x] COFF short import libraries, with DLL dependency and real call tests.
- [x] Standard GNU COFF long import libraries, including MinGW consumer and genuine GNU dlltool fixture execution.
- [ ] Mixed/custom COFF import layouts and delay import tables.
- [x] Mach-O external `N_INDR` forwarding aliases, chains, archive dependencies, and native macOS object/archive calls.
- [x] Mach-O eager indirect pointer binding, local/absolute markers, and standard amd64/arm64 stubs with native scalar/large-record calls.
- [x] ELF amd64 SIZE32/SIZE64 and i386 SIZE32, selected weak/strong/common sizes, archive dependencies, and native Linux calls.
- [x] ELF AArch64 ADR, literal/GOT loads, conditional/test-bit branches, zero GOT addends and unchecked ADRP semantics, with native Linux calls.
- [x] ELF AArch64 MOVW_UABS/SABS/PREL, large-code C objects, ABS16/PREL16, signed-or-unsigned ABS32, and both null relocation encodings.
- [x] ELF AArch64 unresolved weak relative values and call/jump NOPs, preserving selected zero definitions and external providers, with native Linux calls/data.
- [x] Missing weak references cannot satisfy later strong references through cached zero addresses or COFF import slots; native ELF failure/retry tests cover both object orders and archives.
- [x] ELF AArch64 GOT-relative MOVW/data, 15-bit GOT/GOT-page loads, GOTPCREL32 and PLT32, owned reserved bases, and native GCC small-PIC calls.
- [x] ELF amd64 GOT32/GOTOFF64/GOTPC32/GOT64/GOTPCREL64/GOTPC64/GOTPLT64/PLTOFF64, native GCC/Clang large-PIC calls, and shared owned-base rules with i386 GOTPC execution.
- [x] ELF i386 GOT32/GOT32X baseless memory addresses and GNU LEA offsets, with native Clang/GNU assembler calls and system-linked executable comparisons.
- [x] ELF amd64/i386 8/16-bit absolute and PC-relative data fields, signed REL addends, checked widths and native Linux object/archive calls.
- [ ] Remaining relocations on existing targets; Mach-O custom stubs and general coalescing.
- [x] Legacy ELF `.ctors/.dtors` pointer tables, priorities, sentinels, and mixed modern-array execution.
- [x] COFF integer-returning CRT initializers, permanent failure state, and owned exit-registration cleanup.
- [x] Mach-O linker-resolved initializer offsets, original-image rebasing, zero offsets, mixed pointer tables, lazy archives and native macOS Go/llgo execution.
- [x] ELF `.init/.fini` split GNU CRT startup fragments, original archive input order, NOP alignment, lifecycle ordering, and validation retry.
- [x] Optional Windows amd64/arm64 C runtime function tables, OS stack traversal, and owned unregistration.
- [x] Optional Linux amd64/arm64/386 C `.eh_frame` registration with retained libgcc, native stack traversal and deregistration.
- [x] Optional macOS amd64/arm64 C DWARF registration with system libunwind, implicit FDE rebasing, native traversal and deregistration.
- [x] Windows AMD64 version 1 disjoint chained C unwind records, shared primaries, multi-level integer saves, validation retry and native Go/llgo stack traversal.
- [x] Windows AMD64 nested overlapping chains, disjoint registration intervals and read-only resumed-parent proxies, with partial-prolog and register/stack restoration tests.
- [x] Linux amd64/arm64/386 libgcc C++ personality/LSDA, native throw/catch/rethrow, RAII, COMDAT FDE pruning, retirement and object/archive retry tests.
- [x] macOS amd64/arm64 C++ DWARF personality/LSDA with system libunwind, explicit/implicit LSDA rebasing, native exception calls, native header/callback/cache ownership, registry-failure retry and retirement.
- [x] macOS amd64/arm64 compact unwind records, read-only native relative tables/header, frame/frameless/large-stack C++ execution, weak-copy exclusion and mixed DWARF fallback.
- [ ] Windows language handlers and newer unwind versions, additional Mach-O forms and additional POSIX unwind providers.
- [x] Concurrent Symbol/Function/CallPlan calls and reentrant callbacks, with retirement before cleanup and no invocation lock across native calls.
- [x] Scoped reentrant load/link initializers with staged raw symbols, prior OS-library/retained-provider symbols, busy errors for ordinary operations, and concurrent close/failure lifetime guards before publication.
- [ ] Explicit raw-object TLS models, per-thread allocation, and thread destructor ownership.
- [x] Explicit packed record values through compiler-validated C bridges for Go/llgo, ordinary transport records, nested arrays, native pointers, factories and variadic calls without libffi.
- [x] Explicit packed non-bitfield members through validated C bridges, including arrays, nested records, native pointers/factories, concrete variadic calls and packing combined with explicit member alignment.
- [x] Named member-packed bitfields through compiler-validated C bridges, MSVC/MinGW allocation units, packed/aligned members, small/large native records, factories, pointers and variadic calls.
- [x] Member-packed bitfields mixed with unnamed/zero-width declarations in otherwise unpacked Windows records, preserving allocation units, consecutive/trailing boundaries and alignment with real Go/llgo calls.
- [x] Explicit packing on unnamed and zero-width bitfields, including padding-only allocation units, shared units and packed zero-width boundaries with native small-record and nested calls.
- [x] Numeric/omitted GNU alignment on unnamed and zero-width bitfields, including ignored MSVC requests, padding-only units, packed combinations and native Go/llgo calls.
- [x] Compiler-qualified ordinary `#pragma pack` records with nested packing levels, record alignment, arrays, native addresses/function-pointer fields, factories and variadic calls through Go/llgo C bridges.
- [x] Pragma-packed named/unnamed/zero-width bitfields with scoped compiler probes, MSVC/MinGW boundary adaptation, packed combinations, nested arrays and native Go/llgo calls.
- [x] Ordinary pragma-packed over-aligned members/nested values through compiler-measured C bridges, numeric/omitted/type requests, arrays/floating aggregates/native pointer fields, factories and variadic calls.
- [x] Compiler-effective global Clang packing caps with local pragma override/pop, response files/frontend forwarding, MSVC oversized pragma rules, native bitfields/arrays/alignment/pointers/factories and Go/llgo object/archive/library calls.
- [x] Over-cap pragma bitfield combinations with compiler-observed allocation alignment, ignored/shared requests, zero-width boundaries, nested arrays and native Go/llgo object/archive/library calls.
- [x] Small GNU AMD64 interior zero-width gaps through implicit anonymous-prefix padding, including multiple/trailing boundaries, named bitfields, nested records/arrays and fixed/variadic native calls.
- [x] Small GNU AMD64 trailing unnamed nonzero padding without named bitfields through implicit power-of-two tail padding, preserving native size/alignment, register/stack calls, nested arrays, factories and variadic promotions.
- [x] Numeric/omitted scalar, enum and native-address typedef alignment with raised/lowered requests, alias chains, scalar/native-address array leaves, compiler-observed Windows storage placement and native Go/llgo object/archive/library calls.
- [x] Whole-array typedef alignment at independent nested alias layers, including scalar/enum/record/native-address elements, compiler-validated packing and native Go/llgo object/archive/library calls.
- [x] Named aligned scalar/enum typedef bitfields with raised/lowered requests, shared allocation units, packing/member alignment, Linux AArch64 argument alignment and native Go/llgo object/archive/library calls.
- [x] Aligned unnamed/zero-width scalar/enum typedef bitfields with observed unit starts/gaps, ignored/shared boundaries, packing, small SysV prefix/tail facades and native Go/llgo object/archive/library calls.
- [x] Aligned record aliases with accessible canonical bodies, inherited/raised/lowered requests, separate alias/body metadata, compiler calling-ABI checks and native Go/llgo object/archive/library calls.
- [x] Anonymous record bodies accessible only through aligned typedefs, with independent body size/alignment/offset/mask probes, inherited spellings, packing restoration and native Go/llgo execution.
- [x] Plain GNU zero-width pragma boundaries with compiler-observed gaps and root alignment, including lowered record members, floating/HFA values, leading/trailing/consecutive fields, non-power-of-two offsets, GCC checks and native Go/llgo calls.
- [ ] Remaining typedef packing forms, GNU unnamed positive-width root-alignment differences and small GNU AMD64 unnamed gaps with floating fields outside qualified implicit facades.
- [x] Named integral/Boolean bitfields through compiler byte-mask probes and validated Go/llgo C bridges, including packed combinations, nested arrays, native factories and variadic calls.
- [x] Unnamed/zero-width bitfields with compiler-qualified declaration positions, leading/trailing padding, packed combinations, nested arrays, floating fields and native pointer/factory/variadic calls.
- [x] Explicit numeric record/member alignment through validated C bridges, including packed/bitfield combinations, nested arrays, floating aggregates, native factories and variadic calls.
- [x] Compiler-evaluated omitted GNU alignment and type-based `_Alignas` members, including record/typedef/array alignment types, nested records, packed fields, named bitfields and native Go/llgo calls.
- [x] Compiler-qualified named/typedef enums with integer/Boolean transports, packed/short and fixed underlying types, record/array/callback fields, native factories and variadic prefixes through dynamic, cgo and direct llgo adapters.
- [ ] Unions and remaining packing/alignment forms through validated compiler adapters where libffi cannot represent the ABI, including small GNU AMD64 interior gaps mixed with original unnamed nonzero bitfields.
- [x] Dynamic argument counts for scalar/aggregate calls, variadic shapes, and callbacks; reproducible call/allocation benchmarks.
- [x] Session-owned logical-signature CIF reuse, aggregate layout deduplication, and lazy typed pointee layout caches with rollback.
- [x] Indexed signature lookup and canonical omitted/explicit scalar metadata, preserving logical type distinctions.
- [x] Bounded aggregate call buffer reuse, independent nested/concurrent storage, zeroing and owned cleanup.
- [x] Bounded scalar call buffer reuse, independent nested/concurrent storage, foreign value/address clearing, and owned cleanup.
- [x] Bounded native pointee buffer reuse, full-capacity zeroing/range guards, transient owner/type cleanup and shared aggregate byte budgeting.
- [x] Shared physical ABI resources for compatible logical signatures, preserving validation/metadata, bounded buffers and independent retirement.
- [x] GNU/LLVM thin archives, relative paths, regular-member proxies and object snapshots.
- [x] Universal Mach-O FAT32/FAT64 inspection and baseline macOS object/archive/library selection.
- [x] Optional Clang scalar/typedef/opaque-pointer declaration generation, target guards, supported conventions, variadic prefixes and real Go/llgo calls.
- [x] Native ABI layout inspection with independent Go results and compiler `sizeof`/alignment/member-offset execution tests.
- [x] Compiler-validated ordinary record declarations, array members and typed struct-pointer parameters/results, including generated Go/llgo execution.
- [x] Function-pointer parameter/result prototypes, captured scalar/struct/pointer callbacks, native factories, variadic pointer calls and Windows 386 conventions.
- [x] Direct fixed-scalar/opaque-pointer C cdecl llgo adapters, all primitive widths, native pointer lifetimes and generated runtime execution without libffi.
- [x] Direct ordinary-record llgo adapters, fixed array fields, native struct pointers and Clang/llgo layout checks without libffi.
- [x] Direct fixed-cdecl function-pointer parameters/results, native factories and noncapturing same-thread Go entries without libffi.
- [x] Typed scalar/native-address cgo bridges for Go/llgo, concrete variadic shapes with C promotions, native function-pointer prototype forwarding and no-libffi runtime tests.
- [x] Fixed Windows Go 386 stdcall/fastcall cgo bridges, mixed scalars, decorated exports and native function-pointer convention preservation without libffi.
- [x] Direct variadic llgo methods with concrete promoted tails, empty calls, record prefixes/results and native variadic factories/forwarding without libffi.
- [x] Direct llgo narrow-parameter qualification with wide results, signed/unsigned boundaries, Boolean values, native factories/forwarding, C-to-Go callback arguments, stack/sret parameters and fixed variadic prefixes; objects/archives/libraries at `-O0` and `-O2` require llgo's C ABI and debug-pointer fixes.
- [x] Typed by-value cgo records for Go/llgo, recursive field copies, native C layout checks, nested arrays/large returns and record-valued native function pointers without libffi.
- [x] Higher-order/multiple-indirection function pointers, recursive prototype snapshots and generated cgo/llgo factories, consumers and pointer slots.
- [x] Callback record fields and multidimensional entry arrays with cloned prototypes, native-address cgo/llgo records, nested arrays and retained Go callback registrations.
- [x] Explicit C++17 scalar/native-address class manifests generating constructors, ordinary/const/static members and consuming destruction; exact member-pointer calls, secondary-base native virtual dispatch, exception status codes and Go/llgo C bindings without libffi.
- [x] Compiler-generated selected public base views, including multiple/virtual inheritance and shared diamond bases, with borrowed ownership, cleared outputs and rejected private/ambiguous/downcast conversions.
- [x] Explicit C++ class values/references/pointers, native copies and copy exceptions, owned noncopyable C++17 results, borrowed const views and static-only classes with inaccessible destructors.
- [x] Exact C++ native function-pointer arguments/results, constructor retention, native factories and captured Go callback leases with nested reentry and explicit unregister/destruction during retirement.
- [x] Named C++ captured lambda/`std::function` call operators, mutable state and independent copies, move-only capture ownership, native-entry construction, empty-function exceptions and retained Go callbacks during retirement.
- [x] Named C++ scalar/character/void/enum/array/union and multi-level native pointees through existing object-pointer manifests, exact const/level validation, native mutation and borrowed provider retirement.
- [x] Optional compiler-confirmed public/unambiguous base discovery among selected class names/aliases, including transitive/virtual inheritance, conditional compilation and native generated base views.
- [ ] Callable/class/member declaration discovery and inheritance discovery beyond selected names.
- [x] Explicit retained session-to-session function/data symbol providers, transitive code leases and cleanup ordering without libffi.
- [x] Explicit JSON/Go dependency manifests, exact declared ABI versions, multiple-root provider planning, retained import aliases, lazy archive export roots, ordered cleanup and failure rollback without libffi.
- [ ] Automatic provider discovery and version negotiation beyond explicit manifests.
- [x] Owned native scalar/record/array storage with stable address leases, C-retained pointer tests and explicit retirement.
- [x] Generated cgo struct-pointer methods using opaque native addresses, native pointer prototypes and owned storage integration without casting Go record memory.
- [x] Explicit long-lived native registration owners acquiring and coupling value, callback and code leases with caller-supplied unregister/join, cleanup retry and native-thread execution tests.
- [x] Managed module replacement with explicit state migration, generation-scoped calls, failure/panic cleanup and publication before old scope/code-lease retirement; close rejects pending publication without locking across native calls.
- [x] Native Swift/ObjC++ C facades with opaque retain/release, leased Go observers, destruction during session/callback retirement, cleanup retry and macOS Go/llgo execution.
- [ ] Optional ABIBridge-backed Apple C facades for native Swift/ObjC integration, runtime versions, UI threading and device qualification.
- [ ] BSD backends, additional ISAs, and llgo 386 qualification when toolchains/runners are available.
- [x] Optional Go-managed LLVM IR/bitcode object compiler, input snapshots, owned artifacts, cancellation/cleanup, and native call/initializer tests.
- [x] Ordinary GNU/COFF/BSD bitcode/native mixed archive compilation, duplicate member names/order, lazy selection, source/artifact snapshots and owned cleanup.
- [x] Thin/proxy bitcode/native archive compilation, shared source snapshots/budgets, dependency directory metadata, source deletion and Windows DLL calls.
- [x] Explicit LLVM module merging, exact target/layout validation, eager initialization, owned snapshots and linker/compiler cancellation.
- [ ] Separate process/RPC and Wasm executors; GPU/eBPF require domain-specific executors.
