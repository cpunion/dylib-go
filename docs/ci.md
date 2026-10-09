# GitHub CI

Go and llgo use independent workflows and separately compiled test processes:

- [Go workflow](../.github/workflows/go.yml): `actions/setup-go@v7`, Go 1.27.x.
- [llgo workflow](../.github/workflows/llgo.yml): `xgo-dev/setup-llgo@v0.2.0`, llgo revision `b86d349178d610e6b15e95c2a5fdfcb130d09fa1`, built from source, Go 1.27.x, LLVM 22; Windows uses the MinGW profile.
- [README workflow](../.github/workflows/readme.yml): `actions/setup-node@v7`, Node 24, and lockfile-pinned embedme 1.22.1; verifies embedded source matches the README.

Workflows run on pushes to main, pull requests, and manual dispatch. Pure Go jobs set `CGO_ENABLED=0` and run parser, error, and execution-refusal tests in a process compiled for the target architecture.

| Target | Go execution | Pure Go inspection | llgo execution |
| --- | --- | --- | --- |
| Linux amd64 / arm64 | Native runners | Native runners | Native objects and libraries |
| macOS amd64 / arm64 | Intel / Apple Silicon runners | Same runners | Native objects and libraries |
| Windows amd64 | Native runner, Clang/MinGW | Native runner | Native objects and DLLs, MinGW |
| Windows arm64 | Independent gc process tests objects/archives/DLLs and scalar/struct ABI; reuses llgo job's setup-go and C tools | Native ARM64 runner | Objects/archives/DLLs, scalar/struct ABI, and Go/llgo producers |
| Linux 386 | Objects/archives/ELF32 libraries, scalar/struct ABI, and gc c-shared producer; multilib Clang and i386 libffi | 32-bit Go process on amd64 Linux | Not qualified; setup-llgo does not install 386 |
| Windows 386 | Objects/archives/PE32 DLLs, scalar/struct C cdecl ABI, and gc c-shared producer; i386 toolchain and libffi under WoW64 | 32-bit Go process on amd64 Windows | Not qualified; setup-llgo does not install 386 |

All eight Go execution targets and six llgo targets set `DYLIB_TEST_REQUIRE_NATIVE=1`, `DYLIB_TEST_REQUIRE_SHARED=1`, and `DYLIB_TEST_REQUIRE_TOOLS=1`, failing if a required backend or tool is missing. Windows ARM64 runs independent Go and llgo object/archive/DLL suites. Clang generates i386 ELF/COFF and ARM64 COFF fixtures for target-boundary tests: matching hosts accept them, foreign hosts reject execution, and unsupported 32-bit Mach-O is rejected.

## Languages and calling adapters

| Interface / producer | Go host | llgo host |
| --- | --- | --- |
| C, C++ without exceptions/RTTI, archives, libraries | Eight raw-object targets | Six raw-object targets |
| CLI/compiler-probe example `int32(int32,int32)` | `examples/call` cgo adapter | `examples/call` direct C function-pointer adapter |
| Caller-defined `double(int32,double,float,uint64)` | `examples/cgo` on all eight library targets | `examples/llgo` on all six library targets |
| libffi mixed integer/floating-point signatures, variadic promotions/records/pointers, reusable plan lifetime | Eight Go targets; race checks on the five Go native amd64/arm64 jobs (Go race does not support 386) | Six targets; raw COFF constant-pool COMDAT exercised |
| C++ inline COMDAT/shared state from objects and archives | Eight targets | Six targets |
| Windows 386 stdcall/fastcall fixed calls and callback entries | Native Windows 386 C fixtures | Not qualified |
| Fixed C callbacks: scalars/structs, errors, captures, leases, C-created threads with allocation/GC | Eight targets; object/archive/library and native worker fixtures | Six targets; collector-aware foreign-thread entry |
| Foreign ELF/COFF COMDAT discard; COFF SECTION/SECREL; weak external search/aliases | Parser/relocation fixtures independent of host; Windows executes weak overrides and EXACT_MATCH/NEWEST selection | Same fixtures and native Windows execution |
| COFF high section indexes, bigobj, and extended relocation tables | Foreign fixtures on every host; Windows executes objects/archives and checks the final extended relocation | Same fixtures and Windows AMD64/ARM64 execution |
| Concurrent calls and callback reentry | Pure lifetime/retirement tests; native recursive same-plan calls, parallel scalar/struct-pointer calls, close during callbacks, finalizer observations, and C-thread nesting | Same native execution cases on six hosts |
| Universal Mach-O | FAT32/FAT64 foreign inspection and malformed/subtype cases; macOS executes real lipo objects/archives and dylibs with architecture-specific results | Same macOS execution cases |
| Thin archives | Foreign object/proxy inspection, path origins, validation, read budgets and retry; genuine GNU/LLVM archive calls, deleted-file snapshots and proxy execution on eight hosts; Windows DLL origins | Same native cases on six hosts |
| Optional LLVM compiler | Text IR/bitcode native calls; explicit merged modules with cross-module calls, initialization, private symbol isolation, target/layout rejection and linker/compiler cancellation; ordinary/thin/proxy mixed archives with lazy selection, cleanup and README examples; Windows source DLL directories | Same native cases on six hosts; independent Go process on Windows ARM64 |
| C declaration generation | Fresh Clang AST compared with committed target-qualified Go declarations; scalar/float/Boolean/pointer/variadic calls, record layout validation, struct returns and pointer mutation; padded/nested/array/floating-point/large records, variadic record prefixes and mismatch rejection; C long/char probes; Windows 386 conventions | Same generated/parsed native cases on six hosts; independent Go process on Windows ARM64 |
| Native layout queries | Actual C `sizeof`, `_Alignof` and `offsetof` compared with backend layouts from objects/archives/libraries: all scalar widths, Boolean/pointers, padded/nested records and arrays/matrices; independent results, limits and conventions | Same native cases on six hosts; independent Go process on Windows ARM64 |
| Session-owned caches | Different symbol addresses share a plan; concurrent calls, failure isolation, snapshots, logical metadata, raw oversized pointees, layout reuse and allocation rollback | Same native cases on six hosts |
| Dynamic argument storage | 64 scalar/struct arguments and callback parameters; variadic totals 1/32/33/64, zero arguments, count mismatch, and declaration/invocation parsing | Same native cases on six hosts; Go covers 386 |
| COFF long imports | Foreign-target template validation and genuine GNU Binutils fixtures; Windows executes GNU function/data/ordinal imports and MinGW consumers on AMD64/ARM64/386 | Same fixtures and Windows AMD64/ARM64 execution |
| COFF short imports and DLL dependencies | All-target metadata policies; Windows executes genuine LLD `.lib` files, functions/data, name/ordinal imports, paths, handle reuse, and failure/retry | Same metadata and Windows AMD64/ARM64 execution |
| Rust `extern C`, Zig `export`, Fortran `bind(C)` | Both Linux/macOS architectures | Both Linux/macOS architectures |
| Swift C-exported library and raw metadata refusal | Both macOS architectures | Both macOS architectures |
| Go c-shared | Eight library targets, including both 386 hosts | Six library targets |
| llgo c-shared | Local macOS and CI Windows ARM64; other targets use llgo hosts | Six library targets |

These are workflow requirements; inspect [Go](https://github.com/cpunion/dylib-go/actions/workflows/go.yml) / [llgo](https://github.com/cpunion/dylib-go/actions/workflows/llgo.yml) checks for the relevant commit's results. Each producer validates a sample C ABI subset. Windows Rust/Zig/Fortran, Linux Swift, and native ObjC ABI are outside this matrix.

`DYLIB_TEST_LANGUAGES=rust,zig,fortran,swift,go,llgo` selects producers explicitly. A selected compiler that is missing or inappropriate for the host fails. Override executable paths with `DYLIB_RUSTC`, `DYLIB_ZIG`, `DYLIB_FC`, `DYLIB_SWIFTC`, `DYLIB_GO`, and `DYLIB_LLGO`. Go/llgo library calls run in child processes that retain their runtime library references until exit, allowing Windows to remove DLL files afterward.

`bash examples/run.sh go library|llgo` delegates to `scripts/verify-examples.sh`, which builds a complete C library, independently compiles and executes the appropriate typed adapter (`examples/cgo` or `examples/llgo`), and then runs `examples/bind` and `examples/callback` with `-tags libffi`. Each program must print exactly 42. The examples use complete C libraries; separate native tests exercise raw COMDAT selection. The six amd64/arm64 targets execute the typed adapter, dynamic call, and Go capture callback examples with each host compiler; both 386 targets also execute them with Go; the Go Windows ARM64 examples run in the llgo workflow's independent Go step.

`bash examples/run.sh go quickstart|llgo` runs the Linux/macOS object/archive smoke test in `examples/readme/quickstart.sh`. Both compiler workflows execute it on amd64/arm64, and Go also executes it on Linux 386, checking object and archive results and exercising no-cgo inspection. Windows raw-object execution is verified separately by the native test suite and CLI script.

`scripts/verify-cli.sh go|llgo` independently compiles the dynamic CLI with `-tags libffi`. All eight Go native library targets, and six llgo targets, execute Go-style declarations and typed invocations with mixed scalars, small integers, C booleans, integer boundaries, native/temporary pointers, zero arguments, and void returns. Struct and array tests cover by-value struct arguments/results, integer/floating-point arrays, multidimensional arrays, arrays of structs/pointers, C-created callback records, variadic array members, nested padding, large returns, floating-point records, pointer fields, in/out mutation, alias identity, and temporary pointer lifetime checks. All eight raw targets execute scalar, struct, and array-member object/archive calls. The library example suite also builds and runs `examples/arrays` with Go and llgo. `bash examples/run.sh go dynamic|llgo` runs both README input forms through `examples/readme/dynamic.sh` on Linux/macOS. `bash examples/run.sh go structs|llgo` executes `examples/readme/structs.sh` on the six amd64/arm64 targets with both hosts and on both 386 targets with Go. Pure Go parser tests cover all eight inspection jobs, including 386.

README library code blocks are embedded from the executed Go source files under `examples/` with an `<!-- embedme ... -->` marker. The separate README job checks freshness with `npm ci --ignore-scripts` and `npm run readme:verify`; compilation and result checks remain in the native Go and llgo jobs. The short README fixture commands use the same `examples/run.sh <go|llgo> <example>` invocations as the native workflows. The CLI script also tests `examples/run.sh <go|llgo> call` with a struct declaration, a relative library path containing spaces from another directory, and a typed struct pointer loaded from an archive. Script implementations are linked, rather than embedded. To update a Go snippet, edit its source and run `npm run readme` before committing.

## Toolchain limits

[setup-llgo v0.2.0](https://github.com/xgo-dev/setup-llgo/tree/v0.2.0)'s [platform validation](https://github.com/xgo-dev/setup-llgo/blob/v0.2.0/src/platform.ts) accepts amd64/arm64 only, although the compiler has some 386 capabilities. This project provides native Go i386 backends for Linux ELF32 and Windows COFF/PE32, but does not claim llgo 386 execution. An installer or cross-compilation alone cannot qualify a native execution target.

On Linux, llgo v1.0.6 misparses newline-only `pkg-config --cflags libffi` output as `-`, causing Clang to read an extra stdin input and emit two AST JSON documents ([upstream issue #2749](https://github.com/xgo-dev/llgo/issues/2749)). Users of that older version can set `PKG_CONFIG_ALLOW_SYSTEM_CFLAGS=1` as a workaround. The qualified compiler revision includes the upstream fix, so CI no longer needs that setting.

Runner labels follow [GitHub's official list](https://docs.github.com/en/actions/reference/runners/github-hosted-runners): ubuntu-24.04, ubuntu-24.04-arm, macos-15-intel, macos-15, windows-2022, and windows-11-arm.

Raw lifecycle fixtures run in every native Go and llgo job, without requiring libffi: C++ dependency initialization/destruction, ordinary archive selection, C termination tables, `atexit`/`__cxa_atexit`, selective and recursive `__cxa_finalize`, two-image isolation, validation rollback, and close idempotence. A retained observer library verifies callbacks after raw-image release. Pure Go jobs cross-inspect the eight target lifecycle table formats. See [lifecycle coverage](lifecycle.md).

Linux jobs also execute real Clang `-fno-use-init-array` `.ctors/.dtors` output, including 386 Go. Tests cover reversed constructor entries, forward destructor entries, `0/-1` sentinels, normalized priorities mixed with modern arrays, C++ dependency/ordinary-archive selection, retry, and finalizer validation before initialization. All pure Go jobs inspect legacy ELF tables for amd64, arm64, and 386 and check malformed tables and priorities.

Windows jobs execute COFF integer `.CRT$XI*` initialization before void `.CRT$XC*`, including native Go i386 cdecl. Positive/negative errors stop initialization, prevent repeated calls/publication, and retain registered-exit code until `Close`. Tests check dependency/root validation retry, invalid entries, and successful/failed cleanup. POSIX jobs exercise the shared integer-call and ownership machinery through a private test table, without claiming ELF/Mach-O integer-array support. libffi jobs run cleanup callbacks that reenter Go and verify `ErrClosed` without deadlock.

Windows amd64/arm64 Go and llgo jobs execute optional raw runtime function table registration. A DLL observer uses `RtlVirtualUnwind` through two real raw C frames, from objects and archives. Tests verify OS lookup, deletion after close, and ownership during failed C initialization cleanup. Pure Go jobs inspect real AMD64/ARM64 `.pdata/.xdata` producer records and validate malformed/unsupported metadata. See [unwind scope](unwind.md).

Linux amd64/arm64 Go and llgo jobs, and native Go 386, execute optional ELF frame registration with libgcc, `_Unwind_Backtrace` through two raw C frames, and `_Unwind_Find_FDE` before/after close. Object/archive calls, default loading, validation retry and failed-initializer ownership are covered. Pure Go jobs inspect all three ELF producers and reject malformed CIE/FDE/CFI records.

macOS amd64/arm64 Go and llgo jobs execute optional per-FDE DWARF registration with system libunwind, real object/archive stack traversal, before/after-close FDE lookup, validation retry and failed initialization cleanup. Pure Go jobs inspect real `__eh_frame` compiler output and verify explicit ARM64 pairs and implicit x86-64 address rebasing.

CI pins the upstream merge commit of [public C-export foreign-thread guards](https://github.com/xgo-dev/llgo/pull/2752) for dependency packages and executables. The library uses ordinary `//export` declarations and has no private llgo runtime hooks; v1.0.6 cannot qualify C-created-thread callbacks on this path. `setup-llgo` builds the exact commit from source; the full matrix tests that compiler rather than applying patches during a job.
