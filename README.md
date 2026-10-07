# llgo-dylib

[![Go](https://github.com/cpunion/llgo-dylib/actions/workflows/go.yml/badge.svg)](https://github.com/cpunion/llgo-dylib/actions/workflows/go.yml)
[![llgo](https://github.com/cpunion/llgo-dylib/actions/workflows/llgo.yml/badge.svg)](https://github.com/cpunion/llgo-dylib/actions/workflows/llgo.yml)

A Go runtime object loader: load `.o/.obj` files, link required members of `.a/.lib` archives, resolve symbols and relocations, and execute native code in the current process. Shared libraries (`.so/.dylib/.dll`) use the host operating system's loader.

The project reimplements the general loading and linking mechanisms of early [DDL](https://github.com/Marenz/ddl). It uses Go's `debug/elf`, `debug/macho`, and `debug/pe`, with no LLVM C++, ORC, or JITLink dependency. D ABI, ModuleInfo, and D runtime compatibility are outside its scope.

This is an experimental implementation with native execution tests. See the [platform and language matrix](docs/support.md) for its supported subset and limitations.

## Quick start

Use Go 1.23+ and a C compiler. Clang is used below to produce sample inputs; the loader does not require Clang at runtime.

```sh
mkdir -p build
clang -fPIC -c testdata/add.c -o build/add.o
go run ./cmd/ddlgo inspect build/add.o
go run ./cmd/ddlgo call add 20 22 build/add.o
# 42

ar rcs build/add.a build/add.o
go run ./cmd/ddlgo call add 20 22 build/add.a
# 42

# Compile the CLI with llgo to use its direct C function-pointer adapter.
llgo build -o build/ddlgo-llgo ./cmd/ddlgo
build/ddlgo-llgo call add 20 22 build/add.o
```

The CLI's `call` command is a demonstration of the known signature `int32_t(int32_t,int32_t)`. The loader API has no signature-specific binding methods.

Inspection also works without cgo:

```sh
CGO_ENABLED=0 go run ./cmd/ddlgo inspect build/add.o
```

## Generic symbols and caller-defined adapters

`Resolve` returns an untyped symbol handle. `WithAddress` keeps the owning session alive while a caller-defined adapter uses its address:

```go
import dylib "github.com/cpunion/llgo-dylib"

s := dylib.New(dylib.Options{})
defer s.Close()
if err := s.Load("build/plugin.so"); err != nil { panic(err) }
symbol, err := s.Resolve("my_function")
if err != nil { panic(err) }
err = symbol.WithAddress(func(address uintptr) error {
    // Invoke a handwritten or generated adapter with the exact native
    // signature, or access native data with its known layout.
    return myAdapter(address)
})
if err != nil { panic(err) }
```

`myAdapter` is application code. It supplies the actual types and calling convention; symbol names do not reveal function prototypes. Ordinary Go can use a cgo adapter, while llgo can use a caller-defined `//llgo:type C` function-pointer type. See [the cgo example](examples/cgo/main.go) and [the llgo example](examples/llgo/main.go), which call the mixed signature `double(int32_t,double,float,uint64_t)` without enabling the optional dynamic ABI backend.

Run the examples on a supported native host:

```sh
bash scripts/verify-examples.sh go
bash scripts/verify-examples.sh llgo
# Each prints a result of 42.
```

`Load` may be called repeatedly before the first `Link`, `Lookup`, `Resolve`, or `Bind`. Add all dependencies before linking. A failed link can be retried after adding dependencies. A successful link seals the session; create a new session for another plugin set.

## Dynamic signatures

`Bind(name, abi.Signature)` handles caller-supplied scalar signatures through the optional libffi backend. Build with `-tags libffi` and install the libffi development files and pkg-config.

```go
import (
    "math"
    "github.com/cpunion/llgo-dylib/abi"
)

f, err := s.Bind("mixed", abi.Signature{
    Result: abi.F64,
    Args: []abi.Type{abi.I32, abi.F64, abi.F32, abi.U64},
})
if err != nil { panic(err) }
v, err := f.Call(abi.Int32(10), abi.Float64(20.5), abi.Float32(1.5), abi.Uint64(10))
if err != nil { panic(err) }
result := math.Float64frombits(v.Bits) // 42
```

Supported scalar types are `i32/u32/i64/u64/f32/f64/pointer`, with `void` allowed as a result and up to 32 fixed arguments. Aggregate values, variadic calls, native callbacks, C++ `this` adjustment, and Swift calling conventions are not implemented.

```sh
go test -tags libffi ./...
llgo test -tags libffi ./...
```

Ordinary Go's default build does not require libffi. The llgo compiler's own runtime dependencies, including its GC and libffi, are separate from this optional backend.

On Linux with llgo v1.0.6, set `export PKG_CONFIG_ALLOW_SYSTEM_CFLAGS=1` before enabling libffi. This avoids that version's newline-only pkg-config CFLAGS parsing bug ([upstream issue #2749](https://github.com/xgo-dev/llgo/issues/2749)). CI sets the workaround; ordinary Go does not require it.

## Public API

- `Inspect(path)`: read format, target, symbols, and archive metadata without executing code.
- `Load(path)` / `Define(name, address)`: add inputs or native symbols supplied by the host.
- `Link(roots...)`: select archive roots, extract dependencies, and apply relocations.
- `Lookup(name)`: obtain an unmanaged native address; callers manage its lifetime.
- `Resolve(name)`: resolve once and return a session-owned, untyped `Symbol`.
- `Symbol.WithAddress(callback)`: use an address while preventing concurrent `Close`.
- `Bind(name, abi.Signature)` / `Function.Call`: bind and invoke an explicit dynamic scalar signature with libffi.
- `Close()`: free the object image and release system library references; subsequent symbol access and bound calls return `ErrClosed`.

Mach-O names omit one leading linker underscore. C++ names still require their exact mangled linkage name.

## Lifetime and execution boundaries

Inputs must be trusted native code. The parser and execution layer provide no security sandbox. Target ISA, object format, OS ABI, CPU features, and dependencies must match the host. Relocation does not emulate a different CPU or operating system.

Objects are allocated RW, relocated in Go, flushed from the instruction cache, and protected as RX/R/RW per section. No pages are RWX. The raw object path rejects recognized TLS, automatic constructors/destructors, COMDAT, and language runtime registration requirements. It does not register exception unwind information. Use complete shared libraries when these services are needed; the OS handles their dependencies, TLS, and initialization. Exceptions must still remain within the native call boundary.

`Symbol.WithAddress`, `Function.Call`, and `Close` are serialized. An adapter must finish using its address before returning, and must not re-enter locking methods of the same session. Raw addresses, asynchronous native threads, and native callbacks require caller-managed lifetimes. `WithAddress` is a Go-side lifetime guard, not a native callback facility. Go function values cannot be cast into C function pointers; `Define` requires a native function or data address.

Use `Options{KeepLibraries:true}` for Go/llgo `c-shared` libraries and other runtimes with background threads, retaining OS references until process exit. The CLI option is `-keep-libraries`. `ProcessSymbols:true` (CLI `-process`) searches POSIX host exports; on Windows, load the supplying DLL explicitly.

The former experimental `BindInt32`, `CallInt32`, and `Int32Func` APIs have been removed. Migrate known signatures to `Resolve` plus a typed adapter, or use `Bind` with an explicit `abi.Signature` for dynamic calls.

## Source layout and verification

```text
format_*.go, archive.go   Go parsers and the unified object model
linker.go, relocate.go   Symbol selection, archive extraction, layout, relocation
symbol.go, function.go  Generic symbol handles and dynamic signature bindings
abi/                    Go signature descriptions and optional scalar libffi calls
internal/native/        OS memory, instruction cache, and shared-library operations
examples/call/   Fixed-signature adapter used only by the CLI and tests
examples/cgo, llgo/     Caller-defined mixed-signature adapters
cmd/ddlgo/              Inspection and demonstration CLI
testdata/               Native compiler inputs for language probes
docs/                   Design, compatibility, comparisons, CI, and evidence
```

```sh
go test ./...
CGO_ENABLED=0 go test ./...
go test -race -tags libffi ./...
llgo test -tags libffi ./...
# Explicitly selected compilers are required; missing tools fail the test.
DYLIB_TEST_LANGUAGES=rust,zig,fortran,go go test -v ./...
# Full macOS probes, including Swift and llgo-produced libraries.
DYLIB_TEST_LANGUAGES=1 DYLIB_TEST_LLGO=1 go test -v ./...
```

See the [design and DDL mapping](docs/design.md), [ABIBridge / llcppg comparison](docs/comparison.md), [CI matrix](docs/ci.md), [validation evidence](docs/validation.md), and [roadmap](docs/roadmap.md).
