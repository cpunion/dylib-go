# GitHub CI

Go and llgo use independent workflows and separately compiled test processes:

- [Go workflow](../.github/workflows/go.yml): `actions/setup-go@v7`, Go 1.27.x.
- [llgo workflow](../.github/workflows/llgo.yml): `xgo-dev/setup-llgo@v0.2.0`, llgo v1.0.6, Go 1.27.x, LLVM 22; Windows uses the MinGW profile.
- [README workflow](../.github/workflows/readme.yml): `actions/setup-node@v7`, Node 24, and lockfile-pinned embedme 1.22.1; verifies embedded source matches the README.

Workflows run on pushes to main, pull requests, and manual dispatch. Pure Go jobs set `CGO_ENABLED=0` and run parser, error, and execution-refusal tests in a process compiled for the target architecture.

| Target | Go execution | Pure Go inspection | llgo execution |
| --- | --- | --- | --- |
| Linux amd64 / arm64 | Native runners | Native runners | Native objects and libraries |
| macOS amd64 / arm64 | Intel / Apple Silicon runners | Same runners | Native objects and libraries |
| Windows amd64 | Native runner, Clang/MinGW | Native runner | Native objects and DLLs, MinGW |
| Windows arm64 | Independent gc process tests DLL/scalar ABI; reuses llgo job's setup-go and C tools | Native ARM64 runner | DLL/scalar ABI and Go/llgo producers; raw COFF refusal |
| Linux 386 | No 32-bit object execution backend | 32-bit Go process on amd64 Linux | setup-llgo cannot install 386 |
| Windows 386 | No 32-bit object execution backend | 32-bit Go process on amd64 Windows | setup-llgo cannot install 386 |
| macOS 386 | Not applicable | Go has no darwin/386 port; CI checks that boundary | Not applicable |

Raw-object jobs set `DYLIB_TEST_REQUIRE_NATIVE=1` and `DYLIB_TEST_REQUIRE_TOOLS=1`, failing if a required backend or tool is missing. Windows ARM64 is explicitly named `shared` and sets `DYLIB_TEST_REQUIRE_SHARED=1`. Its DLL calls do not imply raw ARM64 COFF relocation support. Clang actually generates 386 and ARM64 COFF objects for metadata and rejection tests.

## Languages and calling adapters

| Interface / producer | Go host | llgo host |
| --- | --- | --- |
| C, C++ without exceptions/RTTI, archives, libraries | Five raw-object targets | Five raw-object targets |
| CLI/compiler-probe example `int32(int32,int32)` | `examples/call` cgo adapter | `examples/call` direct C function-pointer adapter |
| Caller-defined `double(int32,double,float,uint64)` | `examples/cgo` on all six library targets | `examples/llgo` on all six library targets |
| libffi mixed integer/floating-point signatures | Six targets; five raw-object targets also have race checks | Six targets; Windows DLLs let the OS linker handle constant-pool COMDAT |
| Rust `extern C`, Zig `export`, Fortran `bind(C)` | Both Linux/macOS architectures | Both Linux/macOS architectures |
| Swift C-exported library and raw metadata refusal | Both macOS architectures | Both macOS architectures |
| Go c-shared | Six library targets, including Windows ARM64 | Six library targets |
| llgo c-shared | Local macOS and CI Windows ARM64; other targets use llgo hosts | Six library targets |

These are workflow requirements; inspect [Go](https://github.com/cpunion/dylib-go/actions/workflows/go.yml) / [llgo](https://github.com/cpunion/dylib-go/actions/workflows/llgo.yml) checks for the relevant commit's results. Each producer validates a sample C ABI subset. Windows Rust/Zig/Fortran, Linux Swift, and native ObjC ABI are outside this matrix.

`DYLIB_TEST_LANGUAGES=rust,zig,fortran,swift,go,llgo` selects producers explicitly. A selected compiler that is missing or inappropriate for the host fails. Override executable paths with `DYLIB_RUSTC`, `DYLIB_ZIG`, `DYLIB_FC`, `DYLIB_SWIFTC`, `DYLIB_GO`, and `DYLIB_LLGO`. Go/llgo library calls run in child processes that retain their runtime library references until exit, allowing Windows to remove DLL files afterward.

`bash scripts/verify-examples.sh go|llgo` builds a complete C library, independently compiles and executes the appropriate typed adapter (`examples/cgo` or `examples/llgo`), and then runs `examples/bind` with `-tags libffi`. Each program must print exactly 42. Windows uses a DLL to handle COMDAT through the OS linker. All six library targets execute both a typed adapter and the dynamic example with each host compiler; the Go Windows ARM64 examples run in the llgo workflow's independent Go step.

`examples/readme/quickstart.sh` is the README's Linux/macOS object/archive quick start. Both compiler workflows execute it on both architectures, checking object and archive results and exercising no-cgo inspection. It is not a Windows ARM64 object-execution claim.

Every fenced code block in `README.md` is embedded from one of these executed source files with an `<!-- embedme ... -->` marker. The separate README job checks freshness with `npm ci --ignore-scripts` and `npm run readme:verify`; compilation and result checks remain in the native Go and llgo jobs. To update a snippet, edit its source and run `npm run readme` before committing.

## Toolchain limits

[setup-llgo v0.2.0](https://github.com/xgo-dev/setup-llgo/tree/v0.2.0)'s [platform validation](https://github.com/xgo-dev/setup-llgo/blob/v0.2.0/src/platform.ts) accepts amd64/arm64 only, although the compiler has some 386 capabilities. This project has no 32-bit relocation backend; installation or cross-compilation does not establish native object execution.

On Linux, llgo v1.0.6 misparses newline-only `pkg-config --cflags libffi` output as `-`, causing Clang to read an extra stdin input and emit two AST JSON documents ([upstream issue #2749](https://github.com/xgo-dev/llgo/issues/2749)). The llgo workflow sets `PKG_CONFIG_ALLOW_SYSTEM_CFLAGS=1` to retain a system include flag as a temporary workaround. No tests are disabled.

Runner labels follow [GitHub's official list](https://docs.github.com/en/actions/reference/runners/github-hosted-runners): ubuntu-24.04, ubuntu-24.04-arm, macos-15-intel, macos-15, windows-2022, and windows-11-arm.
