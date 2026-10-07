# Implemented scope and future work

The current implementation is a standalone Go native object loader with real call tests: content identification, a shared object model, archive extraction, local/global/weak/common symbols, an explicit relocation subset, adjacent GOT/branch stubs, W^X memory, rollback, generic symbol lifetime guards, OS libraries, caller-defined gc/llgo adapters, and optional dynamic scalar signatures.

Future extensions need explicit implementation and tests:

1. **Windows object coverage:** native Go and llgo execution is verified for AMD64, and DLL calls for ARM64. Further work includes import libraries, COMDAT, SECREL/SECTION, unwind registration, and ARM64 COFF relocation.
2. **Object initialization and exceptions:** implement platform initialization arrays, reverse destructor order, dependency cycles, and failure handling; then register `.eh_frame`, compact unwind, or Windows runtime function tables. Complete OS libraries currently provide those services.
3. **TLS and more relocations:** define each target's TLS model and thread registration/destruction; add ARM64 COFF, RISC-V, LoongArch, and other relocation backends. Parser support alone does not provide execution support.
4. **Archive and object containers:** universal slice selection, thin-archive path handling, and optional LLVM bitcode/IR compilation. OMF and D `.ddl` compatibility remain outside scope.
5. **llcppg dynamic generation:** complement its static `go:linkname` output with adapters using `Resolve` and `WithAddress`, preserving declaration-based ABI validation.
6. **ABIBridge adapters:** expose Apple Swift/ObjC values and calls through opaque C ABI handles. Platform, runtime version, and pointer-authentication coverage need separate tests.
7. **Advanced signatures and callbacks:** optional libffi records, variadic calls, closures, and compiler-generated llgo thunks. Prepared CIFs are not currently cached, and throughput benchmarks are pending. Known signatures can already use caller-defined adapters.
8. **Independent execution backends:** isolated/remote processes, Wasm, and GPU execution need separate interfaces appropriate to their execution models.

Keep the parser and linker in Go and extend llgo or small native adapters when lower-level facilities are required. A future optional LLVM backend should preserve the independent Go backend rather than make LLVM a default dependency.
