# dylib-go

[![Go](https://github.com/cpunion/dylib-go/actions/workflows/go.yml/badge.svg)](https://github.com/cpunion/dylib-go/actions/workflows/go.yml)
[![llgo](https://github.com/cpunion/dylib-go/actions/workflows/llgo.yml/badge.svg)](https://github.com/cpunion/dylib-go/actions/workflows/llgo.yml)
[![README](https://github.com/cpunion/dylib-go/actions/workflows/readme.yml/badge.svg)](https://github.com/cpunion/dylib-go/actions/workflows/readme.yml)

A Go runtime object loader: load `.o/.obj` files, link required members of `.a/.lib` archives, resolve symbols and relocations, and execute native code in the current process. Shared libraries (`.so/.dylib/.dll`) use the host operating system's loader.

The project reimplements the general loading and linking mechanisms of early [DDL](https://github.com/Marenz/ddl). It uses Go's `debug/elf`, `debug/macho`, and `debug/pe`, with no LLVM C++, ORC, or JITLink dependency. D ABI, ModuleInfo, and D runtime compatibility are outside its scope.

This is an experimental implementation with native execution tests. See the [platform and language matrix](docs/support.md) for its supported subset and limitations.

## Quick start

Use Go 1.23+ and a C compiler. The module path is `github.com/cpunion/dylib-go`; its Go package name is `dylib`. Clang is used below to produce sample inputs; the loader does not require Clang at runtime.

On Linux or macOS amd64/arm64, run `bash examples/readme/quickstart.sh` from a checkout. Pass `llgo` as its argument to build the CLI with llgo. CI executes both variants and checks that the object and archive calls return 42:

<!-- embedme examples/readme/quickstart.sh -->

```sh
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
compiler=${1:-go} # Pass llgo to build the CLI with its direct C-call adapter.
case "$compiler" in
  go|llgo) ;;
  *) echo 'usage: quickstart.sh [go|llgo]' >&2; exit 1 ;;
esac

mkdir -p build
"${CLANG:-clang}" -fPIC -c testdata/add.c -o build/add.o
"$compiler" build -o build/ddlgo ./cmd/ddlgo
build/ddlgo inspect build/add.o
result=$(build/ddlgo call add 20 22 build/add.o)
test "$result" = 42
echo "object result: $result"

"${AR:-ar}" rcs build/add.a build/add.o
result=$(build/ddlgo call add 20 22 build/add.a)
test "$result" = 42
echo "archive result: $result"

# Metadata inspection also works without cgo.
CGO_ENABLED=0 go run ./cmd/ddlgo inspect build/add.o

```

The CLI's plain `call add 20 22 FILE` form retains the original `int32_t(int32_t,int32_t)` demonstration. Caller-supplied signatures use the dynamic CLI below. The loader API has no signature-specific binding methods. Windows users can run the shared-library examples below; see the [platform matrix](docs/support.md) for object execution limits.

## Dynamic CLI signatures

Build the CLI with `-tags libffi` and install libffi development files and pkg-config. It accepts a Go-style declaration followed by positional values, or a compact invocation containing typed values. Quote the whole declaration or invocation so the shell passes parentheses and spaces unchanged. Both forms below return 42; CI executes this script with Go and llgo on Linux/macOS amd64/arm64:

<!-- embedme examples/readme/dynamic.sh -->

```sh
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
compiler=${1:-go} # Pass llgo to build the same dynamic CLI with llgo.

mkdir -p build
"${CLANG:-clang}" -fPIC -c testdata/add.c -o build/add.o
"$compiler" build -tags libffi -o build/ddlgo ./cmd/ddlgo

result=$(build/ddlgo call "func add(int32,int32)int32" 20 22 build/add.o)
test "$result" = 42
echo "declaration result: $result"

# Quote the whole invocation so parentheses and spaces reach the CLI.
result=$(build/ddlgo call "add(20:int32 22:int32)int32" build/add.o)
test "$result" = 42
echo "typed invocation result: $result"

```

The `abi/signature` package parses declarations with Go's standard `go/parser`. Supported spellings are `int32`, `uint32`, `int64`, `uint64`, `float32`, `float64`, `uintptr`, `unsafe.Pointer`, and `*T` for opaque native pointers. Omit the result for a C `void` return. Calls support up to 32 fixed arguments and one result. `int`, `uint`, strings, slices, structures, variadic parameters, and multiple results are rejected. Parameter names and grouped parameters such as `a, b int32` are accepted in declarations.

Integer values accept Go literal bases and underscores. Pointer values are native addresses or `nil`; the CLI does not allocate or marshal pointee storage. Float results retain their precision, unsigned results retain all high bits, and void calls print `void`. The declaration must match the actual native C ABI; parsing does not infer or verify prototypes from object symbols. See the [CLI reference](docs/cli.md) for complete rules and verification coverage.

## Generic symbols and caller-defined adapters

`Resolve` returns an untyped symbol handle. `WithAddress` keeps the owning session alive while a caller-defined adapter uses its address. This complete [cgo example](examples/cgo/main.go) calls the C signature `double(int32_t,double,float,uint64_t)` and prints 42:

<!-- embedme examples/cgo/main.go -->

```go
//go:build cgo && (darwin || linux || windows)

// This example implements a caller-defined signature with an ordinary cgo
// adapter. The loader itself does not know the signature.
package main

/*
#include <stdint.h>
static double call_mixed(uintptr_t address, int32_t a, double b, float c, uint64_t d) {
    return ((double (*)(int32_t, double, float, uint64_t))address)(a, b, c, d);
}
*/
import "C"

import (
	"fmt"
	"os"

	dylib "github.com/cpunion/dylib-go"
)

func run(path string) error {
	s := dylib.New(dylib.Options{})
	defer s.Close()
	if err := s.Load(path); err != nil {
		return err
	}
	symbol, err := s.Resolve("mixed")
	if err != nil {
		return err
	}
	var result float64
	err = symbol.WithAddress(func(address uintptr) error {
		result = float64(C.call_mixed(C.uintptr_t(address), 10, 20.5, 1.5, 10))
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Println(result)
	return s.Close()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mixed-cgo OBJECT_OR_LIBRARY")
		os.Exit(1)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

```

The adapter supplies the actual types and calling convention; symbol names do not reveal function prototypes. With llgo, the [same call](examples/llgo/main.go) uses a caller-defined `//llgo:type C` function-pointer type:

<!-- embedme examples/llgo/main.go -->

```go
//go:build llgo && cgo && (darwin || linux || windows)

// This example uses a caller-defined C signature without the libffi backend.
package main

import (
	"fmt"
	"os"
	"unsafe"

	dylib "github.com/cpunion/dylib-go"
)

// llgo emits the indirect call using the native C calling convention.
//
//llgo:type C
type mixedFunc func(int32, float64, float32, uint64) float64

func run(path string) error {
	s := dylib.New(dylib.Options{})
	defer s.Close()
	if err := s.Load(path); err != nil {
		return err
	}
	symbol, err := s.Resolve("mixed")
	if err != nil {
		return err
	}
	var result float64
	err = symbol.WithAddress(func(address uintptr) error {
		function := *(*mixedFunc)(unsafe.Pointer(&address))
		result = function(10, 20.5, 1.5, 10)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Println(result)
	return s.Close()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mixed-llgo OBJECT_OR_LIBRARY")
		os.Exit(1)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

```

Neither typed adapter enables the optional dynamic ABI backend. CI compiles and runs these programs against a real C shared library on Linux, macOS, and Windows, on amd64 and arm64. Run `bash scripts/verify-examples.sh go` or `bash scripts/verify-examples.sh llgo` to reproduce the typed and dynamic examples; libffi development files are required for the dynamic example.

`Load` may be called repeatedly before the first `Link`, `Lookup`, `Resolve`, or `Bind`. Add all dependencies before linking. A failed link can be retried after adding dependencies. A successful link seals the session; create a new session for another plugin set.

## Dynamic signatures

`Bind(name, abi.Signature)` handles caller-supplied scalar signatures through the optional libffi backend. Build with `-tags libffi` and install the libffi development files and pkg-config. This complete [dynamic example](examples/bind/main.go) is compiled and executed with both Go and llgo in CI, returning 42 on all six native library targets:

<!-- embedme examples/bind/main.go -->

```go
//go:build libffi && cgo && (darwin || linux || windows)

// This example supplies a dynamic scalar signature to the libffi backend.
package main

import (
	"fmt"
	"math"
	"os"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
)

func run(path string) error {
	s := dylib.New(dylib.Options{})
	defer s.Close()
	if err := s.Load(path); err != nil {
		return err
	}
	f, err := s.Bind("mixed", abi.Signature{
		Result: abi.F64,
		Args:   []abi.Type{abi.I32, abi.F64, abi.F32, abi.U64},
	})
	if err != nil {
		return err
	}
	v, err := f.Call(abi.Int32(10), abi.Float64(20.5), abi.Float32(1.5), abi.Uint64(10))
	if err != nil {
		return err
	}
	fmt.Println(math.Float64frombits(v.Bits))
	return s.Close()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mixed-bind OBJECT_OR_LIBRARY")
		os.Exit(1)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

```

Supported scalar types are `i32/u32/i64/u64/f32/f64/pointer`, with `void` allowed as a result and up to 32 fixed arguments. Aggregate values, variadic calls, native callbacks, C++ `this` adjustment, and Swift calling conventions are not implemented.

Run `go test -tags libffi ./...` or `llgo test -tags libffi ./...` to exercise the dynamic backend.

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

| Path | Purpose |
| --- | --- |
| `format_*.go`, `archive.go` | Go parsers and the unified object model |
| `linker.go`, `relocate.go` | Symbol selection, archive extraction, layout, relocation |
| `symbol.go`, `function.go` | Generic symbol handles and dynamic signature bindings |
| `abi/` | Go signature descriptions and optional scalar libffi calls |
| `abi/signature/` | Pure Go declaration, typed-invocation, and scalar literal parsing |
| `internal/native/` | OS memory, instruction cache, and shared-library operations |
| `examples/call/` | Fixed-signature adapter used only by the CLI and tests |
| `examples/cgo/`, `examples/llgo/`, `examples/bind/` | Executable README examples |
| `examples/readme/` | Executable README quick-start and dynamic CLI scripts |
| `cmd/ddlgo/` | Inspection and demonstration CLI |
| `testdata/` | Native compiler inputs for language probes |
| `docs/` | Design, compatibility, comparisons, CI, and evidence |

Run `scripts/verify.sh` for ordinary Go, no-cgo, and vet checks. Set `DYLIB_TEST_FFI=1 DYLIB_TEST_LLGO=1` to add race/libffi and llgo tests. Set `DYLIB_TEST_LANGUAGES=rust,zig,fortran,go` for those compiler probes, or `DYLIB_TEST_LANGUAGES=1 DYLIB_TEST_LLGO=1` for all macOS probes, including Swift and llgo-produced libraries. Explicitly selected compilers are required; missing tools fail the test.

All fenced code in this README comes from source files under `examples/`, embedded with [embedme](https://github.com/zakhenry/embedme). Edit those files, run `npm ci --ignore-scripts` followed by `npm run readme`, and commit the source and regenerated README together. The README workflow runs `npm run readme:verify` to reject stale snippets; the Go and llgo workflows execute the examples and check their results.

See the [design and DDL mapping](docs/design.md), [ABIBridge / llcppg comparison](docs/comparison.md), [CI matrix](docs/ci.md), [validation evidence](docs/validation.md), and [roadmap](docs/roadmap.md).
