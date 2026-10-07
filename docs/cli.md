# CLI declarations and dynamic calls

`ddlgo inspect FILE` reads metadata without loading native code. `ddlgo call` loads the supplied files into a session, resolves a named entry, and invokes it with an explicit C ABI signature. Options `-process` and `-keep-libraries` precede the declaration or invocation. Inputs must match the host's architecture and operating-system ABI.

## Input forms

| Form | Example | Arguments and files |
| --- | --- | --- |
| Go-style declaration | `"func add(int32,int32)int32"` | Follow with `20 22 FILE...` |
| Typed invocation | `"add(20:int32 22:int32)int32"` | Follow with `FILE...` |
| Typed invocation with commas | `"add(20:int32,22:int32)int32"` | Follow with `FILE...` |
| Original demonstration | `add` | Follow with `20 22 FILE...`; exact `int32(int32,int32)` signature |

Quote declarations and invocations as a single shell argument. Unquoted parentheses may be interpreted or rejected by the shell before the CLI runs. The [executable README script](../examples/readme/dynamic.sh) demonstrates the exact build and call commands and checks their results.

Dynamic forms require a CLI compiled with `go build -tags libffi` or `llgo build -tags libffi`, plus libffi development files and pkg-config. Without the optional backend, the original demonstration still uses its typed cgo/llgo adapter; dynamic forms report a build instruction before loading any files. With libffi enabled, every form uses `Session.Bind` and `Function.Call`.

## Declaration grammar

The standalone `abi/signature` package uses `go/parser` and `go/ast` to parse one named function declaration without a body. Named or unnamed parameters, grouped names, an optional single named result, and Go whitespace are supported. An absent result denotes C `void`. A symbol name must be a Go identifier; an Itanium C++ mangled name can be used if it is a valid identifier. The parser neither demangles nor inspects a function's actual prototype.

| Go spelling | ABI type | Input / output |
| --- | --- | --- |
| `int32` | `abi.I32` | Signed 32-bit integer |
| `uint32` | `abi.U32` | Unsigned 32-bit integer |
| `int64` | `abi.I64` | Signed 64-bit integer |
| `uint64` | `abi.U64` | Unsigned 64-bit integer |
| `float32` | `abi.F32` | IEEE single precision |
| `float64` | `abi.F64` | IEEE double precision |
| `uintptr` | `abi.U32` or `abi.U64` | Unsigned integer matching the host word size, for C `uintptr_t` |
| `unsafe.Pointer`, `*T` | `abi.Pointer` | Opaque native address; pointee types do not cause marshaling |
| Omitted result | `abi.Void` | No result; CLI prints `void` |

Explicit-width integers avoid assuming a C `int`, `long`, or `size_t` width from a Go type name. `int`, `uint`, `bool`, smaller integers, aggregates, strings, slices, maps, interfaces, function values, receivers, generic parameters, variadic functions, and multiple results are outside the current dynamic scalar backend. Pointer spellings describe an address only; they do not translate Go objects into a native layout or supply callbacks.

`signature.Parse` returns a `Declaration` with `Name` and `Signature`, suitable for `session.Bind(declaration.Name, declaration.Signature)`. `signature.ParseCall` also returns parsed `Args`. `ParseValue` and `FormatValue` convert scalar literals and native result bit patterns independently of libffi, so parsing works in no-cgo builds and 32-bit inspection processes too.

## Literal and lifetime rules

Declarations take exactly their parameter count as positional values, followed by one or more input files. Typed invocations put each `value:type` in parentheses, separated by whitespace or commas. They do not evaluate Go expressions. Named parameters in a declaration do not change the positional order.

Integers accept decimal, hexadecimal, binary, octal, and Go underscores; leading zero follows Go octal syntax. Signed and unsigned overflow, negative unsigned values, malformed floats, unsupported types, and missing arguments/files are rejected before a library can be loaded. The original demonstration preserves its decimal-only argument parsing.

Pointers accept a numeric native address or `nil`. Addresses must remain valid for the call, and pointee memory is caller-managed. No Go objects are pinned or allocated. Results are printed in decimal for integers, round-trip precision for floats, hexadecimal for pointers, and `void` for no result. Symbols and calls remain under the session's existing lifetime guards.

## Execution evidence

`scripts/verify-cli.sh go|llgo` builds a real C library and a separate CLI executable. It exercises both dynamic forms, the original form, mixed integer/floating-point registers, negative results, unsigned high bits, float32 results, null pointers, zero arguments, and void results. Both hosts run on all six native library targets: Linux/macOS/Windows amd64 and arm64. Raw object and archive calls additionally run on the five supported raw targets; Windows ARM64 explicitly uses DLLs only.

Parser and literal tests run in all Go CI jobs, including no-cgo Linux/Windows 386 processes, and in the llgo jobs. Passing grammar tests on 386 does not establish native execution there. The README dynamic script runs on both Linux/macOS architectures with both compilers, and embedme checks that the displayed commands match its source.
