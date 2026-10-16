# C union member facades

`compiler/cunion.Generate` renders C17 facades for explicitly selected scalar,
pointer or ordinary struct members of native union arguments and results.
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
automatic active-member detection. Pointer views borrow their pointees and
provider lifetime; retain an appropriate code or registration lease when an
address outlives an ordinary call.

Compile `Facade.CSource` against the original header with the producer target
and flags. It uses Clang/GCC's union classification builtin, C17 `_Generic`
prototype and member checks, and full-member storage checks. Wrong prototypes,
non-union typedefs, missing/incompatible members and bitfields fail compilation.
This is an explicit declaration manifest, not header discovery. C++ native
unions and other language object semantics need a separate adapter.

`Facade.CHeader` exposes C transport declarations. Struct transports include the
original header so `compiler/clang` can inspect their actual fields and layouts;
scalar-only headers remain independent. Parse it with `compiler/clang`, then use
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
unrepresentable transport records. Bare array/union member views, nested member
paths, bitfields, variadic functions and non-default native conventions are not
implemented. C exceptions or other runtime unwinding cannot cross the facade.

Generation is pure Go and supports at most 256 exports, 64 arguments per export
and 1 MiB of source. Native tests compile the provider and facade as separate
translation units, then execute objects, archives and shared libraries at `-O0`
and `-O2`. The Go and llgo jobs cover integer boundaries, floating NaN payloads
and negative zero, Boolean values, borrowed/null pointers,
small/large/packed/aligned unions, ordinary struct member views with nested
arrays/pointers, and provider retirement. Cross-target tests compile all eight
supported native targets, generate cgo bindings for all eight, and qualify
direct llgo bindings only on amd64/arm64. See [the example
manifest](../examples/cunion/manifest.go).
