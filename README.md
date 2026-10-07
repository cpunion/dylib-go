# dylib-go

[![Go](https://github.com/cpunion/dylib-go/actions/workflows/go.yml/badge.svg)](https://github.com/cpunion/dylib-go/actions/workflows/go.yml)
[![llgo](https://github.com/cpunion/dylib-go/actions/workflows/llgo.yml/badge.svg)](https://github.com/cpunion/dylib-go/actions/workflows/llgo.yml)
[![README](https://github.com/cpunion/dylib-go/actions/workflows/readme.yml/badge.svg)](https://github.com/cpunion/dylib-go/actions/workflows/readme.yml)

A Go runtime object loader: load `.o/.obj` files, link required members of `.a/.lib` archives, resolve symbols and relocations, and execute native code in the current process. Shared libraries (`.so/.dylib/.dll`) use the host operating system's loader.

The project reimplements the general loading and linking mechanisms of early [DDL](https://github.com/Marenz/ddl). It uses Go's `debug/elf`, `debug/macho`, and `debug/pe`, with no LLVM C++, ORC, or JITLink dependency. D ABI, ModuleInfo, and D runtime compatibility are outside its scope.

This is an experimental implementation with native execution tests. See the [platform and language matrix](docs/support.md) for its supported subset and limitations.

## Platform, architecture, and format support

The execution columns describe native loading and calls on the listed host. Inspection only reads metadata and does not execute code.

| Platform | Architecture | Object execution | Archive execution | Shared library execution | CI coverage |
| --- | --- | --- | --- | --- | --- |
| Linux | amd64 | ELF64 little-endian RELA `.o` | Ordinary GNU/BSD ar `.a` | ELF `.so` | Go and llgo native calls |
| Linux | arm64 | ELF64 little-endian RELA `.o` | Ordinary GNU/BSD ar `.a` | ELF `.so` | Go and llgo native calls |
| macOS | amd64 | Mach-O 64 `.o` | Ordinary GNU/BSD ar `.a` | Mach-O `.dylib` | Go and llgo native calls |
| macOS | arm64 | Mach-O 64 `.o` | Ordinary GNU/BSD ar `.a` | Mach-O `.dylib` | Go and llgo native calls |
| Windows | amd64 | AMD64 COFF `.obj` / `.o` | Ordinary COFF ar `.lib` / `.a` | PE `.dll` | Go and llgo native calls |
| Windows | arm64 | ARM64 COFF `.obj` / `.o` | Ordinary COFF ar `.lib` / `.a` | PE `.dll` | Go and llgo native calls |
| Linux | 386 | ELF32 little-endian REL/RELA `.o` | Ordinary GNU/BSD ar `.a` | ELF32 `.so` | Native 32-bit Go calls on amd64 runners |
| Windows | 386 | i386 COFF `.obj` / `.o` | Ordinary COFF ar `.lib` / `.a` | PE32 `.dll` | Native 32-bit Go calls under WoW64 |
| macOS | 386 | Unsupported | Unsupported | Unsupported | No Go `darwin/386` port |

The six amd64/arm64 targets test scalar and struct calls with both Go and llgo; Linux/Windows 386 use native 32-bit Go processes. The current setup-llgo installer accepts only amd64/arm64, so no llgo 386 execution is claimed. Ordinary Go execution requires cgo; metadata inspection does not. Raw objects and archives support the implemented relocation subset; TLS, automatic constructors/destructors, COMDAT, and exception unwinding are not supported on that path. Shared libraries use the host OS loader. Inputs and call signatures must match the host architecture and OS ABI; this table does not imply cross-CPU or cross-OS execution.

Metadata inspection uses pure Go and can read ELF, Mach-O, COFF/PE, and ordinary ar files independently of the file's CPU architecture. Thin archives, fat Mach-O, COFF import libraries/bigobj, OMF, D `.ddl`, Go gc `.a`, and LLVM IR/bitcode are not directly supported library inputs. See the [detailed format limits and language matrix](docs/support.md) for other platforms and unsupported features.

## Installation

Add the module with `go get github.com/cpunion/dylib-go`. The package name is `dylib`. Use Go 1.23+ and a C compiler for native execution, or build the host with llgo. Pure Go metadata inspection works with `CGO_ENABLED=0`.

Known native signatures can use caller-defined cgo or llgo adapters. For dynamic signatures, install libffi development files and pkg-config and build with `-tags libffi`. The library API does not depend on the test CLI. Clang produces the CI fixtures, but is not needed to load already compiled inputs at runtime.

## Public API

- `Inspect(path)`: read format, target, symbols, and archive metadata without executing code.
- `Load(path)` / `Define(name, address)`: add inputs or native symbols supplied by the host.
- `Link(roots...)`: select archive roots, extract dependencies, and apply relocations.
- `Lookup(name)`: obtain an unmanaged native address; callers manage its lifetime.
- `Resolve(name)`: resolve once and return a session-owned, untyped `Symbol`.
- `Symbol.WithAddress(callback)`: use an address while preventing concurrent `Close`.
- `Bind(name, abi.Signature)` / `Function.Call`: bind and invoke an explicit dynamic signature with libffi.
- `Close()`: free the object image and release system library references; subsequent symbol access and bound calls return `ErrClosed`.

Mach-O and i386 COFF C names omit one leading linker underscore. i386 stdcall suffixes remain; the dynamic backend uses C cdecl. C++ names still require their exact mangled linkage name.

## Library usage with typed adapters

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

Neither typed adapter enables the optional dynamic ABI backend. CI compiles and runs these programs against a real C shared library with Go on eight targets and llgo on the six amd64/arm64 targets. Run `bash scripts/verify-examples.sh go` or `bash scripts/verify-examples.sh llgo` to reproduce the typed and dynamic examples; libffi development files are required for the dynamic example.

`Load` may be called repeatedly before the first `Link`, `Lookup`, `Resolve`, or `Bind`. Add all dependencies before linking. A failed link can be retried after adding dependencies. A successful link seals the session; create a new session for another plugin set.

## Dynamic signatures

`Bind(name, abi.Signature)` handles caller-supplied signatures through the optional libffi backend. Build with `-tags libffi` and install the libffi development files and pkg-config. This complete [dynamic example](examples/bind/main.go) is compiled and executed in CI, returning 42 with Go on all eight native library targets and llgo on the six amd64/arm64 targets:

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

Supported scalar types are signed/unsigned 8/16/32/64-bit integers, C `bool`, `f32/f64`, and pointers. Ordinary C structs support arguments, results, nesting, and pointer fields through `abi.TypeDesc`. Use `abi.StructValue` for fields, `abi.AddressOf` for temporary native copies of scalars or structs, and `abi.Ptr` for caller-managed native addresses. Temporary mutations are copied back; native code must not retain those pointers. Layout and register classification come from libffi. Calls allow `void` results and up to 32 fixed arguments. Arrays, unions, packed structs, bitfields, variadic calls, native callbacks, C++ `this` adjustment, and Swift calling conventions are not implemented.

`abi/signature.Parse` converts a Go-style declaration into a symbol name and `abi.Signature`, ready for `Bind`; `ParseCall` also parses typed values for dynamic tests. The parser uses Go's standard `go/parser` and does not infer prototypes from symbols.

Run `go test -tags libffi ./...` or `llgo test -tags libffi ./...` to exercise the dynamic backend.

Ordinary Go's default build does not require libffi. The llgo compiler's own runtime dependencies, including its GC and libffi, are separate from this optional backend.

On Linux with llgo v1.0.6, set `export PKG_CONFIG_ALLOW_SYSTEM_CFLAGS=1` before enabling libffi. This avoids that version's newline-only pkg-config CFLAGS parsing bug ([upstream issue #2749](https://github.com/xgo-dev/llgo/issues/2749)). CI sets the workaround; ordinary Go does not require it.

## Lifetime and execution boundaries

Inputs must be trusted native code. The parser and execution layer provide no security sandbox. Target ISA, object format, OS ABI, CPU features, and dependencies must match the host. Relocation does not emulate a different CPU or operating system.

Objects are allocated RW, relocated in Go, flushed from the instruction cache, and protected as RX/R/RW per section. No pages are RWX. The raw object path rejects recognized TLS, automatic constructors/destructors, COMDAT, and language runtime registration requirements. It does not register exception unwind information. Use complete shared libraries when these services are needed; the OS handles their dependencies, TLS, and initialization. Exceptions must still remain within the native call boundary.

`Symbol.WithAddress`, `Function.Call`, and `Close` are serialized. An adapter must finish using its address before returning, and must not re-enter locking methods of the same session. Raw addresses, asynchronous native threads, and native callbacks require caller-managed lifetimes. `WithAddress` is a Go-side lifetime guard, not a native callback facility. Go function values cannot be cast into C function pointers; `Define` requires a native function or data address.

Use `Options{KeepLibraries:true}` for Go/llgo `c-shared` libraries and other runtimes with background threads, retaining OS references until process exit. `ProcessSymbols:true` searches POSIX host exports; on Windows, load the supplying DLL explicitly.

The former experimental `BindInt32`, `CallInt32`, and `Int32Func` APIs have been removed. Migrate known signatures to `Resolve` plus a typed adapter, or use `Bind` with an explicit `abi.Signature` for dynamic calls.

## Source layout and verification

| Path | Purpose |
| --- | --- |
| `format_*.go`, `archive.go` | Go parsers and the unified object model |
| `linker.go`, `relocate*.go` | Symbol selection, archive extraction, layout, relocation |
| `symbol.go`, `function.go` | Generic symbol handles and dynamic signature bindings |
| `abi/` | Go signature descriptions and optional scalar/struct libffi calls |
| `abi/signature/` | Pure Go declaration, typed-invocation, and typed literal parsing |
| `internal/native/` | OS memory, instruction cache, and shared-library operations |
| `examples/call/` | Fixed-signature adapter used only by the CLI and tests |
| `examples/cgo/`, `examples/llgo/`, `examples/bind/` | Executable README examples |
| `examples/readme/` | Executable README test CLI scripts |
| `cmd/ddlgo/` | Test CLI for inspection and native calls |
| `testdata/` | Native compiler inputs for language probes |
| `docs/` | Design, compatibility, comparisons, CI, and evidence |

Run `scripts/verify.sh` for ordinary Go, no-cgo, and vet checks. Set `DYLIB_TEST_FFI=1 DYLIB_TEST_LLGO=1` to add race/libffi and llgo tests. Set `DYLIB_TEST_LANGUAGES=rust,zig,fortran,go` for those compiler probes, or `DYLIB_TEST_LANGUAGES=1 DYLIB_TEST_LLGO=1` for all macOS probes, including Swift and llgo-produced libraries. Explicitly selected compilers are required; missing tools fail the test.

All fenced code in this README comes from source files under `examples/`, embedded with [embedme](https://github.com/zakhenry/embedme). Edit those files, run `npm ci --ignore-scripts` followed by `npm run readme`, and commit the source and regenerated README together. The README workflow runs `npm run readme:verify` to reject stale snippets; the Go and llgo workflows execute the examples and check their results.

See the [design and DDL mapping](docs/design.md), [ABIBridge / llcppg comparison](docs/comparison.md), [CI matrix](docs/ci.md), [validation evidence](docs/validation.md), and [roadmap](docs/roadmap.md).

## Test CLI

`cmd/ddlgo` is a testing tool for metadata inspection and native calls using the library API. Its `-keep-libraries` flag corresponds to `Options.KeepLibraries`, and `-process` to `Options.ProcessSymbols`.

### Object and archive smoke test

On Linux amd64/arm64/386 or macOS amd64/arm64, run `bash examples/readme/quickstart.sh` from a checkout. Pass `llgo` as its argument to build the CLI with llgo. CI executes both variants and checks that the object and archive calls return 42:

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
source scripts/native-cflags.sh
"${CLANG:-clang}" ${native_cflags[@]+"${native_cflags[@]}"} -fPIC -c testdata/add.c -o build/add.o
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

The CLI's plain `call add 20 22 FILE` form retains the original `int32_t(int32_t,int32_t)` demonstration. Caller-supplied signatures use the dynamic CLI below. The loader API has no signature-specific binding methods. Windows object/archive calls are also exercised by the CLI tests; see the [platform matrix](docs/support.md) for relocation and runtime limits.

### Dynamic CLI signatures

Build the CLI with `-tags libffi` and install libffi development files and pkg-config. It accepts a Go-style declaration followed by positional values, or a compact invocation containing typed values. Quote the whole declaration or invocation so the shell passes parentheses and spaces unchanged. Both forms below return 42; CI executes this script with Go and llgo on Linux/macOS amd64/arm64, and with Go on Linux 386:

<!-- embedme examples/readme/dynamic.sh -->

```sh
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
compiler=${1:-go} # Pass llgo to build the same dynamic CLI with llgo.

mkdir -p build
source scripts/native-cflags.sh
"${CLANG:-clang}" ${native_cflags[@]+"${native_cflags[@]}"} -fPIC -c testdata/add.c -o build/add.o
"$compiler" build -tags libffi -o build/ddlgo ./cmd/ddlgo

result=$(build/ddlgo call "func add(int32,int32)int32" 20 22 build/add.o)
test "$result" = 42
echo "declaration result: $result"

# Quote the whole invocation so parentheses and spaces reach the CLI.
result=$(build/ddlgo call "add(20:int32 22:int32)int32" build/add.o)
test "$result" = 42
echo "typed invocation result: $result"

```

The `abi/signature` package parses declarations with Go's standard `go/parser`. Supported spellings are fixed-width integers from `int8`/`uint8` through `int64`/`uint64`, `byte`, `rune`, `bool`, `float32`, `float64`, `uintptr`, `unsafe.Pointer`, `*T`, and inline ordinary C structs. Omit the result for a C `void` return. Calls support up to 32 fixed arguments and one result. `int`, `uint`, strings, slices, arrays, unions, packed structs, variadic parameters, and multiple results are rejected. Parameter names and grouped parameters such as `a, b int32` are accepted in declarations.

Integer values accept Go literal bases and underscores. Pointer values accept native addresses or `nil`; known pointee types also accept temporary literals, as shown below. Float results retain their precision, unsigned results retain all high bits, and void calls print `void`. The declaration must match the actual native C ABI; parsing does not infer or verify prototypes from object symbols. See the [CLI reference](docs/cli.md) for complete rules and verification coverage.


#### Struct values and pointers

Inline `struct{...}` descriptions support ordinary C structs as arguments and results, nested fields, and pointer fields. A known pointee type accepts a temporary literal such as `&40` or `&{a:20,b:22}`. The backend allocates native storage, copies fields in, and copies mutations back after the call. Layout, padding, and register classification come from libffi rather than Go's struct layout.

This script runs with both Go and llgo on Linux/macOS/Windows amd64 and arm64, and with Go on Linux/Windows 386. Its source is embedded by embedme:

<!-- embedme examples/readme/structs.sh -->
```sh
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
compiler=${1:-go} # Pass llgo to build with llgo instead.
source scripts/native-cflags.sh
mkdir -p build
binary=build/ddlgo
case "$(uname -s)" in
  Darwin)
    library=build/structs.dylib
    "${CLANG:-clang}" -dynamiclib testdata/cli.c -o "$library"
    ;;
  Linux)
    library=build/structs.so
    "${CLANG:-clang}" ${native_cflags[@]+"${native_cflags[@]}"} -shared -fPIC testdata/cli.c -o "$library"
    ;;
  MINGW*|MSYS*)
    binary=build/ddlgo.exe
    library=build/structs.dll
    "${CLANG:-clang}" ${native_cflags[@]+"${native_cflags[@]}"} -shared testdata/cli.c -Wl,--export-all-symbols -o "$library"
    ;;
  *) echo 'unsupported example host' >&2; exit 1 ;;
esac
"$compiler" build -tags libffi -o "$binary" ./cmd/ddlgo

# Pass an ordinary C struct by value using a Go-style declaration.
result=$("$binary" call "func sum_pair(struct{a,b int32})int32" "{a:20,b:22}" "$library")
test "$result" = 42
echo "struct value result: $result"

# The known pointee type allows a temporary native copy of this literal.
result=$("$binary" call "sum_pair_ptr(&{a:20,b:22}:*struct{a,b int32})int32" "$library")
test "$result" = 42
echo "struct pointer result: $result"

# Mutations are copied back; a returned temporary pointer becomes a snapshot.
result=$("$binary" call "mutate_pair(&{20,22}:*struct{a,b int32})*struct{a,b int32}" "$library")
test "$result" = '&{a:22,b:20}'
echo "modified struct result: $result"

```

Native code must not retain a temporary pointer beyond the call. See [literal and lifetime rules](docs/cli.md) for details and remaining limits.
