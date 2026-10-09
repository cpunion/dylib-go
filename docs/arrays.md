# Fixed-length C arrays

`abi.TypeDesc{Type: abi.Array, Len: N, Elem: &element}` describes contiguous C array storage. Arrays are supported as struct members and pointer elements. Element types can be scalars, pointers, ordinary structs, or another fixed-length array. Structs containing arrays can be passed and returned by value, including through C callbacks and concrete variadic calls. A bare array argument or result is rejected: C array parameters decay to pointers, and C functions cannot return arrays by value.

Use `abi.ArrayValue(description, elements...)` with exactly `Len` values, or `abi.Zero(description)` after validation. `Aggregate.Fields` holds array elements by index, just as it holds struct fields in declaration order. Combine it with `abi.StructValue` for an array member. The [complete library example](../examples/arrays/main.go) runs with both Go and llgo through `examples/run.sh <go|llgo> library` in CI.

`abi.AddressOf(&arrayValue)` supplies a temporary native copy for a pointer parameter. Reusing the same value pointer preserves alias identity, and native mutations are copied back after the call. Returned pointers to the whole temporary array become logical `Pointee` values. Interior pointers, or a differently typed view of the same address, cannot escape temporary storage. For example, a returned `*int32` pointing at the first element of a temporary `*[2]int32` is rejected despite sharing its address. Use caller-owned native storage for retained pointers. Arrays of pointers obey the same rules recursively.

Callback array members are decoded into logical values. Their pointer elements remain borrowed native addresses. Callback results may contain those addresses, but cannot contain `AddressOf` temporary pointees, including nested inside arrays; invalid results produce a zero native value and set `Callback.Err`.

## Signature and literal syntax

| C storage | Go-style signature type | Example literal |
| --- | --- | --- |
| `struct { int32_t values[2]; }` | `struct{values [2]int32}` | `{values:{20,22}}` |
| `int32_t (*)[2]` | `*[2]int32` | `&{20,22}` |
| `struct { float values[2][2]; }` | `struct{values [2][2]float32}` | `{{{1,2},{3,4}}}` |
| `struct { int32_t *values[2]; }` | `struct{values [2]*int32}` | `{{&20,&22}}` |

Lengths must be positive integer literals; Go bases and underscores are accepted. Array literals use `{...}` with elided types. Omitted elements are zero-filled. Integer keys set the next index, so `{1:20,22}` fills elements 1 and 2; duplicate and out-of-range indexes are rejected. Formatting prints every element in index order. Slices, zero-length/flexible arrays, inferred lengths, constant expressions, and string-to-byte-array conversion are unsupported.

Descriptors allow at most eight nesting levels, 32 fields per struct, and 65,536 expanded scalar/pointer elements per aggregate. A pointer's annotated element is checked independently. These limits are validated before allocating logical values; native storage is additionally limited to 64 KiB per value.

## Native layout and coverage

The backend represents an array as a libffi aggregate with repeated element types, as described by the [libffi array documentation](https://github.com/libffi/libffi/blob/master/doc/libffi.texi). libffi supplies size, alignment, offsets, and host ABI classification. Array elements share one prepared layout. The same marshaling path handles calls and callbacks without treating Go arrays as C memory.

Use [`abi.LayoutOf`](layout.md) to query array storage size, alignment and the offset of every direct element. A nested array's offsets describe its rows; query the row descriptor separately for its elements. This storage query does not make arrays valid by-value C parameters or results.

The existing native CI matrix executes arrays with Go on Linux AMD64/ARM64/386, macOS AMD64/ARM64, and Windows AMD64/ARM64/386; llgo covers the six AMD64/ARM64 targets. Tests load real C objects, archives, and OS libraries. C-side checksums and C-created callback values verify integer/floating-point arrays, padding, multidimensional arrays, arrays of structs, and arrays longer than 32 elements. Pointer tests cover mutation, aliases, signature ownership, incompatible/interior temporary pointers, and callback pointer lifetimes. CLI tests execute both declaration and typed-invocation forms. This extends the dynamic C ABI; it does not add object formats, relocation kinds, or foreign operating-system ABIs.
