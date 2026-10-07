# Validation record

Date: 2026-10-07. These results describe actual runs with specific toolchains. Re-run the tests for a different compiler or host.

## Native GitHub runners

[PR #1](https://github.com/cpunion/dylib-go/pull/1), final commit `96de660`, passed all 19 checks: [13 Go jobs](https://github.com/cpunion/dylib-go/actions/runs/37567616074) and [6 llgo jobs](https://github.com/cpunion/dylib-go/actions/runs/37567616142). The matrix includes both architectures of Linux/macOS, Windows AMD64 object/archive/DLL calls, and Windows ARM64 DLL/scalar calls with independent Go and llgo hosts. Raw ARM64 COFF execution remains rejected.

Go's five raw-object jobs run functional, language, race, and vet checks. Eight no-cgo jobs run inspection/refusal tests, including real 32-bit Go processes on Linux/Windows. See the [CI matrix](ci.md) and current PR checks for changes made after this baseline. The generic symbol API and mixed-signature examples have their own lifetime and execution checks in the current suite.

## Original local environments

| Environment | Compiler / execution | Result |
| --- | --- | --- |
| macOS arm64 | Go 1.27.0, Clang 22.1.8 | Default tests, no-cgo inspection, and go vet passed |
| macOS arm64 | Go race/libffi, libffi 3.8.0 | Lifetime/concurrency and mixed scalar ABI passed |
| macOS arm64 | Local llgo devel, LLVM 22.1.8 | Default/libffi tests and direct C function-pointer calls passed |
| macOS amd64 | `GOARCH=amd64 CGO_ENABLED=1 CC='clang -arch x86_64'`, Rosetta | Core execution, archives, libraries passed; this local run was not physical Intel hardware |
| Linux arm64 | ARM64 Docker, Go 1.26.5, Clang 22.1.8 | Go libffi tests passed |
| Linux amd64 | AMD64 Docker, Go 1.27.0, Clang 22, architecture translation on ARM64 host | Go libffi tests passed |
| Windows amd64 cross-build | `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go test -c` | Build passed; this local check did not execute Windows code |
| Linux arm64 no-cgo cross-build | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -c` | Build passed; not an additional execution result |

Local logs are in ignored `build/verify-*.log`. Original Docker runs used existing local environments:

- `llgo-pr2404-linux22-arm64:local`, image `a75eaf43ac8f567f0d5727b8a5c9a1827cdf7e524f36908cb2535a9ddfdc0cf7`.
- `llgo-dev-llvm22-amd64:e2e`, image `e09da790c4bfb31a6b1a7354d4c28529da074fd4679f7064e3794758c8497eca`.

These are local identifiers, not prerequisites for other users. A corresponding environment needs Go, Clang, ar, and C development tools; libffi tests additionally need its development files. The original container checks did not compile the host with llgo; GitHub CI subsequently covers that path.

`otool -L` showed that the ordinary Go CLI links system libresolv/libSystem, without LLVM or libffi. The llgo CLI links its toolchain's libffi, BDW GC, libc++, and system libraries, without LLVM. Caller-defined llgo C function-pointer adapters do not use the optional `abi/libffi.go` backend.

## Behavior verified

Tests compile real inputs in temporary directories and check actual results:

- Standalone object `add(20,22)==42`.
- Data, BSS, absolute pointers, common symbols, and cross-object references.
- Lazy archive extraction; an unused member deliberately has an unresolved dependency.
- Objects call symbols from explicitly loaded libraries, optional libc exports, and manual `Define` entries.
- Missing dependencies can be added and linking retried; duplicate strong symbols, unknown names, and roots fail.
- Out-of-section and unsupported relocations fail without publishing an image; corrected inputs can be retried.
- Idempotent `Close`, closed symbol/binding refusal, callback error propagation, and serialization of symbol access with `Close`.
- Real TLS, constructor, and Swift registration objects are rejected.
- Six foreign ELF/Mach-O/COFF targets are generated and inspected. Incompatible targets cannot execute; iOS objects cannot masquerade as macOS plugins.
- Absolute pointer relocation is checked for five format/architecture combinations without executing foreign machine code.
- Real byte/word/dword store instructions exercise Mach-O x86-64 SIGNED_1/2/4 addends.
- GNU and COFF long archive names are checked with real objects, later offsets, and malformed references.
- Optional libffi tests cover mixed integer/floating-point registers, negative results, 64-bit high bits, argument mismatches, and closed calls.
- Caller-defined cgo and llgo examples invoke `double(int32_t,double,float,uint64_t)` through `Symbol.WithAddress` and return 42.
- Fixed native callbacks cover scalar widths, pointers, void/zero-argument entries, nested/large/floating-point struct results, handler failures and panic recovery, retained registrations, lease retirement, and Windows 386 stdcall/fastcall. C-created threads allocate Go memory and trigger GC concurrently. Ordinary Go uses cgo transitions; llgo uses compiler-generated public C-export thread transitions.
- The dynamic `examples/bind` program invokes the same mixed signature through libffi with both Go and llgo. `examples/callback` passes a captured Go closure to a C caller and prints 42 with both compilers. `examples/run.sh <go|llgo> call ARGS...` builds the test CLI and preserves caller argument boundaries and relative paths; the runner also dispatches the library, object/archive, dynamic-signature, and struct examples; CI uses the same commands shown in the README. Library Go snippets are embedded from their executed source files, with a separate freshness check.

## Original language producers

The initial macOS ARM64 run used `DYLIB_TEST_LANGUAGES=1 DYLIB_TEST_LLGO=1 go test -v ./...`:

| Producer | Version | Input / result |
| --- | --- | --- |
| Rust | rustc 1.98.0 | no_std, extern C, panic=abort object returned 42 |
| Zig | 0.16.0 | export fn object returned 42 |
| Fortran | GNU Fortran 16.2.0 | bind(C) object returned 42 |
| C++ | Clang 22.1.8 | Exception/RTTI-free function with a C wrapper returned 42 |
| Swift | Apple Swift 6.3.3 | Raw registration object rejected; C-exported dylib returned 42 |
| Go gc | 1.27.0 | c-shared / export function returned 42 with runtime reference retained |
| llgo | Local devel | Same Go plugin source built as c-shared and returned 42 |

These establish the tested C ABI subset, rather than every language feature, compiler option, or runtime combination. Explicitly selected missing compilers now fail. Unselected languages do not count as coverage. Current platform requirements and per-commit outcomes are described by the CI matrix and Actions checks.

## Reproduction

```sh
scripts/verify.sh
DYLIB_TEST_FFI=1 DYLIB_TEST_LLGO=1 scripts/verify.sh
DYLIB_TEST_LANGUAGES=1 DYLIB_TEST_LLGO=1 go test -v ./...
bash scripts/verify-examples.sh go
bash scripts/verify-examples.sh llgo
GOARCH=amd64 CGO_ENABLED=1 CC='clang -arch x86_64' go test -v ./...
```

The last command targets macOS with Rosetta; native hosts run the normal commands. Set `CLANG` to select the test Clang executable. On Linux with llgo v1.0.6 and libffi, also set `PKG_CONFIG_ALLOW_SYSTEM_CFLAGS=1` as documented in the CI matrix.
