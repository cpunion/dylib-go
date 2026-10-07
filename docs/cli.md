# CLI declarations and dynamic calls

`ddlgo inspect FILE` reads metadata without loading native code. `ddlgo call` loads the supplied files into a session, resolves a named entry, and invokes it with an explicit C ABI signature. Options `-process` and `-keep-libraries` precede the declaration or invocation. Inputs must match the host's architecture and operating-system ABI.

## Input forms

| Form | Example | Arguments and files |
| --- | --- | --- |
| Go-style declaration | `"func add(int32,int32)int32"` | Follow with `20 22 FILE...` |
| Typed invocation | `"add(20:int32 22:int32)int32"` | Follow with `FILE...` |
| Typed invocation with commas | `"add(20:int32,22:int32)int32"` | Follow with `FILE...` |
| Struct by value | `"func sum_pair(struct{a,b int32})int32"` | Follow with `"{a:20,b:22}" FILE...` |
| Struct pointer | `"sum_pair_ptr(&{a:20,b:22}:*struct{a,b int32})int32"` | Follow with `FILE...` |
| Original demonstration | `add` | Follow with `20 22 FILE...`; exact `int32(int32,int32)` signature |

Quote declarations and invocations as a single shell argument. Unquoted parentheses may be interpreted or rejected by the shell before the CLI runs. The executable README scripts for [scalar calls](../examples/readme/dynamic.sh) and [struct calls](../examples/readme/structs.sh) demonstrate exact build/call commands and check their results.

Dynamic forms require a CLI compiled with `go build -tags libffi` or `llgo build -tags libffi`, plus libffi development files and pkg-config. Without the optional backend, the original demonstration still uses its typed cgo/llgo adapter; dynamic forms report a build instruction before loading any files. With libffi enabled, every form uses `Session.Bind` and `Function.Call`.

## Declaration grammar

The standalone `abi/signature` package uses `go/parser` and `go/ast` to parse one named function declaration without a body. Named or unnamed parameters, grouped names, an optional single named result, and Go whitespace are supported. An absent result denotes C `void`. A symbol name must be a Go identifier; an Itanium C++ mangled name can be used if it is a valid identifier. The parser neither demangles nor inspects a function's actual prototype.

| Go spelling | ABI type | Input / output |
| --- | --- | --- |
| `int8`, `uint8`, `byte` | `abi.I8`, `abi.U8` | Signed/unsigned 8-bit integer; `byte` aliases `uint8` |
| `int16`, `uint16` | `abi.I16`, `abi.U16` | Signed/unsigned 16-bit integer |
| `int32`, `rune`, `uint32` | `abi.I32`, `abi.U32` | Signed/unsigned 32-bit integer; `rune` aliases `int32` |
| `int64`, `uint64` | `abi.I64`, `abi.U64` | Signed/unsigned 64-bit integer |
| `bool` | `abi.Bool` | C `_Bool`/`bool`; literals `true` and `false` |
| `float32`, `float64` | `abi.F32`, `abi.F64` | IEEE single/double precision |
| `uintptr` | `abi.U32` or `abi.U64` | Host word size, for C `uintptr_t` |
| `unsafe.Pointer`, `*T` | `abi.Pointer` | Native address or a temporary value when the element type is known |
| `struct{a,b int32}` | `abi.Struct` plus `abi.TypeDesc` | Ordinary C struct with fields in declaration order |
| Omitted result | `abi.Void` | No result; CLI prints `void` |

Explicit-width integers avoid assuming a C `int`, `long`, or `size_t` width from a Go type name. C `bool` is different from Windows `BOOL`, which requires the matching integer type. `int`, `uint`, strings, slices, maps, interfaces, function values, receivers, generic parameters, variadic functions, and multiple results are rejected.

Struct fields must be explicitly named. Grouped fields, nested structs, and pointer fields are supported, with 1–32 fields per struct, at most eight nesting levels, and native storage limited to 64 KiB per value. Layout and padding follow the host's default C ABI through libffi, not Go's memory layout. Struct arguments and results use libffi's native register/stack classification. Arrays, unions, packed structs, bitfields, embedded fields, and empty structs are not supported. Named external struct types cannot be resolved by value; describe them inline. `*UnknownName` remains an opaque pointer accepting only an address or `nil`.

`signature.Parse` returns a `Declaration` with `Name` and `Signature`, suitable for `session.Bind(declaration.Name, declaration.Signature)`. `signature.ParseCall` also returns parsed `Args`. `ParseValue` handles scalars, `ParseTypedValue` handles descriptor-aware values, and `FormatValue` prints results. Parsing has no libffi dependency and works in no-cgo builds and 32-bit inspection processes too.

## Literal and lifetime rules

Declarations take exactly their parameter count as positional values, followed by one or more input files. Typed invocations put each `value:type` in parentheses, separated by whitespace or commas. They do not evaluate Go expressions. Named parameters in a declaration do not change positional order.

Integers accept decimal, hexadecimal, binary, octal, and Go underscores; leading zero follows Go octal syntax. Signed/unsigned overflow, negative unsigned values, malformed floats, unsupported types, and missing arguments/files are rejected before a library can be loaded. The original demonstration preserves its decimal-only argument parsing.

Struct literals accept named fields (`{b:22,a:20}`) or a complete positional list (`{20,22}`). Omitted named fields are zero-filled; `{}` is a zero value for a nonempty struct. Nested field values elide their type, as in `{tag:1,p:{a:20,b:20},extra:1}`. Unknown/duplicate fields and mixed named/positional fields are rejected. Result fields are printed in declaration order.

Pointers accept a numeric native address or `nil`. Such addresses must remain valid for the call and their storage remains caller-managed. With a known element type, `&40:*int32` and `&{a:20,b:22}:*struct{a,b int32}` allocate temporary native copies; struct pointer literals may omit `&`. Pointer fields accept the same forms, including `{p:&{a:20,b:22}}`. No raw Go pointers are passed to native code.

At the library level, `abi.StructValue` constructs logical field values and `abi.AddressOf(&value)` passes a temporary native copy, updating `value` after the call. Reusing the same `*abi.Value` preserves pointer identity within that call. Temporary storage is freed when the call finishes: native code must not retain these pointers. A returned pointer to a whole temporary pointee is represented by `Value.Pointee`, a logical snapshot with its updated fields, rather than a native address. Returning an interior temporary pointer or an incompatible typed view reports an error. This also applies to pointer fields copied back or returned in a struct. Native side effects have already occurred when such a return-value error is reported. Use `abi.Ptr` with caller-managed native storage when the pointer must outlive the call.

Results print decimal integers, `true`/`false`, round-trip precision floats, hexadecimal native pointers, `{field:value,...}` structs, `&value` temporary snapshots, or `void`. Symbols and calls remain under the session's existing lifetime guards. Concurrent callers must synchronize access to shared mutable pointee values.

## Execution evidence

`scripts/verify-cli.sh go|llgo` builds a real C library and a separate CLI executable. Both hosts run on the six Linux/macOS/Windows amd64/arm64 targets. Native Go also runs in Linux/Windows 386 processes, with 32-bit C libraries and Go c-shared producers. Tests cover both dynamic forms, the original form, mixed integer/floating-point registers, integer boundaries, small scalars, booleans, null/temporary pointers, zero arguments, and void results. Struct tests cover arguments/results, nested padding, large returns, floating-point records, pointer fields, in/out mutation, alias identity, signature metadata copying, and temporary pointer lifetime errors. Raw scalar and struct object/archive calls run on all eight supported raw targets, including Windows ARM64 and Linux/Windows 386.

Parser and literal tests run in all Go CI jobs, including no-cgo Linux/Windows 386 processes, and in llgo jobs. Separate cgo-enabled 386 jobs execute native objects and libraries; parsing alone is not execution evidence. The embedded struct script runs on six amd64/arm64 targets with both compilers and on both 386 targets with Go; the raw-object dynamic script runs on Linux/macOS. Embedme verifies that README commands match their executed source.
