# C union facades

`compiler/cunion.Generate` renders C17 facades for explicitly selected scalar,
pointer, ordinary struct or fixed array members of native union arguments and results.
`Value{Type: abi.F64, Union: "union Number", Member: "d"}` transports a Go
`float64`; the facade creates or receives the actual native union. `Type:
abi.Struct, Record: "struct Pair"` selects a struct transport, including
ordinary nested records, fixed array fields and borrowed pointer fields. A
typedef may replace either tagged name. The producer's C compiler supplies union
layout, register classification, hidden result pointers and stack alignment. Go
does not reinterpret a union as an ordinary struct.

`Function.Name` selects the original C function; `Symbol` names a distinct C
export. Every argument/result selects either a scalar/native address, a named
struct, or the corresponding explicitly selected union member. Argument unions
are zero-initialized before assigning the selected member. The caller must
select a valid member for native results; there is no inferred discriminant or
automatic active-member detection. `Member` also accepts dot-separated paths,
such as `payload.inner.pair.a`, through nested struct and union fields. The
selected leaf must exactly match the transport type. The entire input union is
zero-initialized before assigning that leaf, including neighboring fields.
Paths permit at most 16 identifiers and 1024 bytes; pointer dereferences,
array indexing and expressions are rejected. Pointer views borrow their pointees and
provider lifetime; retain an appropriate code or registration lease when an
address outlives an ordinary call.

`Value{Type: abi.Array, Array: "IntPair", Union: "union Arrays", Member: "ints"}`
selects a fixed array typedef such as `typedef int32_t IntPair[2]`. The public
facade uses a generated ordinary struct containing that array in its `values`
field; generated Go bindings expose `Values`. Each typedef shares one wrapper
across argument/result positions and exports, including separately generated
headers. C copies the complete selected array without array-to-pointer decay.
The array extent and element types must match exactly. Pointer/scalar aliases,
empty or incomplete arrays and incompatible extents/elements fail compilation.
Arrays cannot be ordinary native C by-value arguments/results; this selection
requires a union member. Nested paths and multidimensional/record/pointer array
elements use the same interface. Pointer elements remain borrowed.

`Value{Type: abi.Array, Union: "union Payload", Bytes: 32}` selects all 32
bytes of a union object representation. Leave `Member` empty for the root union,
or supply a path selecting a complete nested union. `Bytes` must be 1–65536 and
excludes `Array`/`Record`. The public transport is an ordinary struct containing
`uint8_t values[Bytes]`; generated Go bindings expose `Values`. Equal byte widths
share a wrapper across exports and independently generated headers for the same
original include. Byte-only facade headers do not need the original native header.

The C compiler verifies that both the native root and selected leaf are unions
and the byte count equals the selected union's complete `sizeof`. C copies the
representation without interpreting a member, then calls the original union
prototype. Nested input views explicitly clear all neighboring bytes, including
padding, before copying the selected union. Scalars, structs, arrays, pointers,
bitfields and missing fields cannot masquerade as a union leaf. The caller owns
the meaning of the bytes, valid native representations and any encoded pointer
lifetimes; no active-member detection or C++ object construction is supplied.
Only ordinary byte-array transports cross Go's dynamic or generated ABI boundary.

Compile `Facade.CSource` against the original header with the producer target
and flags. It uses Clang/GCC's union classification builtin, C17 `_Generic`
prototype and member checks, and full-member storage checks. Wrong prototypes,
non-union typedefs, missing/incompatible members and bitfields fail compilation.
This is an explicit declaration manifest, not header discovery. C++ native
unions and other language object semantics need a separate adapter.

`Facade.CHeader` exposes C transport declarations. Struct/array transports include the
original header so `compiler/clang` can inspect their actual fields and layouts;
scalar-only and byte-only headers remain independent. Parse it with `compiler/clang`, then use
`CgoSource`, `LLGoSource` or `ForHost` with `Session.Bind`. Generated cgo and
direct llgo callers require no libffi. Dynamic calls use the facade's supported
scalar/record signature with libffi. Direct binding to the original by-value
union prototype remains unsupported. Both headers and native sources must
describe the same ABI.

The generated exports use the host C/cdecl convention. Selected members support
signed/unsigned 8/16/32/64-bit integers, Boolean, float/double and `void *`
values. The native union may contain other fields, be packed, be explicitly
aligned or exceed register size; those details remain inside compiled C.
Ordinary struct views use the existing compiler/clang layout validation and
backend limits; dynamic/direct llgo calls still reject packed or otherwise
unrepresentable transport records. Array wrappers use those same backend layout
and element-count limits. Bitfield member views,
variadic functions and non-default native conventions are not
implemented. C exceptions or other runtime unwinding cannot cross the facade.

Generation is pure Go and supports at most 256 exports, 64 arguments per export
and 1 MiB of source. Native tests compile the provider and facade as separate
translation units, then execute objects, archives and shared libraries at `-O0`
and `-O2`. The Go and llgo jobs cover integer boundaries, floating NaN payloads
and negative zero, Boolean values, borrowed/null pointers,
small/large/packed/aligned unions, ordinary struct member views with nested
arrays/pointers, nested scalar/struct/pointer leaves at nonzero offsets, zeroed
neighboring fields, named array views with scalar/record/floating/pointer elements,
multidimensional arrays, exact-size whole-union byte views, nested union byte
leaves with zeroed neighboring storage, composable wrapper headers, and provider retirement.
Cross-target tests compile all eight
supported native targets, generate cgo bindings for all eight, and qualify
direct llgo bindings only on amd64/arm64. See [the example
manifest](../examples/cunion/manifest.go).
