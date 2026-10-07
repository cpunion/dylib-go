# Implemented scope and future work

The current implementation is a standalone Go native object loader with real call tests: content identification, a shared object model, archive extraction, local/global/weak/common symbols, an explicit relocation subset, adjacent GOT/branch stubs, W^X memory, rollback, generic symbol lifetime guards, OS libraries, caller-defined gc/llgo adapters, and optional dynamic scalar/struct signatures.

The CLI now accepts Go-style signature declarations and typed invocations through the independent `abi/signature` package. It supports fixed-width scalars, pointers, and ordinary C structs as values and pointees. These ABI improvements do not broaden object relocation or language runtime coverage.

## What complete coverage would require

"Complete" needs a bounded target, object format, ABI, toolchain, and runtime feature set. More parser cases or signature spellings cannot establish arbitrary native execution.

| Coverage area | Work still required |
| --- | --- |
| Current 64-bit raw targets | Remaining relocations, additional COMDAT rules/weak aliases, initialization, TLS, and unwinder registration |
| Windows ARM64 raw objects | Additional COMDAT rules, weak aliases, import-library handling, initialization, and unwind registration |
| 386 and additional ISAs | 386 executes C cdecl on Linux/Windows with Go; Windows 386 stdcall/fastcall are implemented; other conventions and llgo qualification remain. Additional ISAs need linker/ABI backends and execution tests |
| BSD and Apple mobile hosts | Native OS backend, ABI/runtime integration, platform policy compatibility, and device tests |
| Additional containers | Import libraries, bigobj, thin archives, universal target selection; optional IR compilation |
| Full dynamic C/C++ interfaces | Arrays, unions, packed structs, bitfields, callbacks, and generated declaration validation |
| Swift, ObjC, and other language ABIs | Dedicated metadata, ownership, calling-convention, initialization, and runtime adapters |
| Foreign CPU or OS execution | An appropriate emulator/system process plus RPC; native relocation cannot supply OS services |
| Wasm, GPU, eBPF | Separate executors, drivers, or kernel loaders, with independent interfaces and tests |

Complete shared libraries already use the host OS loader for their dependency, initialization, TLS, and native unwind metadata. Calls still require correct ABI adapters, language runtime contracts, and lifetimes. There is no current claim that every language feature works merely because its image loads.

## Implementation priorities

Future extensions need explicit implementation and tests:

1. **Windows object coverage:** native Go and llgo objects, archives, and DLLs are covered for AMD64/ARM64, plus Go for i386. Further work includes import libraries, EXACT_MATCH/NEWEST COMDAT, weak aliases, unwind registration, and additional architecture-specific relocations.
2. **Object initialization and exceptions:** implement platform initialization arrays, reverse destructor order, dependency cycles, and failure handling; then register `.eh_frame`, compact unwind, or Windows runtime function tables. Complete OS libraries currently provide those services.
3. **TLS and more relocations:** define each target's TLS model and thread registration/destruction; extend ARM64 COFF coverage and add RISC-V, LoongArch, and other relocation backends. Parser support alone does not provide execution support.
4. **Archive and object containers:** universal slice selection, thin-archive path handling, and optional LLVM bitcode/IR compilation. OMF and D `.ddl` compatibility remain outside scope.
5. **llcppg dynamic generation:** complement its static `go:linkname` output with adapters using `Resolve` and `WithAddress`, preserving declaration-based ABI validation.
6. **ABIBridge adapters:** expose Apple Swift/ObjC values and calls through opaque C ABI handles. Platform, runtime version, and pointer-authentication coverage need separate tests.
7. **Advanced signatures and callbacks:** extend the implemented libffi records with arrays and additional aggregate forms; add closures and compiler-generated llgo thunks. Variadic promotion, Windows 386 stdcall/fastcall, and owned reusable CIFs/layouts are implemented; cross-binding caches and throughput benchmarks remain pending. Known signatures can already use caller-defined adapters.
8. **Independent execution backends:** isolated/remote processes, Wasm, and GPU execution need separate interfaces appropriate to their execution models.

Keep the parser and linker in Go and extend llgo or small native adapters when lower-level facilities are required. A future optional LLVM backend should preserve the independent Go backend rather than make LLVM a default dependency.
