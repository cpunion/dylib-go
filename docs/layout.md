# Native storage layouts

`abi.LayoutOf(description, convention)` queries the actual native backend used by
dynamic calls and C callbacks. Build with cgo and `-tags libffi`; a valid query in
a build without that backend returns `abi.ErrUnavailable`. Invalid descriptors,
void and invalid conventions fail before backend preparation.

| Result | Meaning |
| --- | --- |
| `Layout.Size` | Storage bytes, including trailing padding |
| `Layout.Alignment` | Required alignment in bytes |
| `Layout.Offsets` | Direct struct fields or array elements, in declaration order |

Offsets are absent for scalars and pointers. For nested records, query each
member's descriptor separately. A matrix's direct offsets describe rows; querying
the row descriptor gives column offsets. The returned slice belongs to the caller;
changing it cannot modify another query, prepared call or callback. Temporary
native type descriptions are released before the function returns, so no `Close`
operation is needed.

Pointer queries describe the native address itself. An annotated pointee is
validated logically but is not laid out or allocated by that query. Arrays can be
queried as storage, including arrays of records and pointers. C still cannot pass
or return a bare array by value. Descriptor nesting/element limits and the
backend's 64 KiB aggregate storage limit remain unchanged.

`abi.Default` and `abi.CDecl` use the host's default C ABI. `abi.StdCall` and
`abi.FastCall` are available on Windows 386; other host/convention combinations
fail through the existing backend convention selection. The query does not
select a foreign operating system, CPU or compiler ABI.

Use these values to compare a compiler's `sizeof`, `_Alignof` and `offsetof`
results before preparing a generated record binding. Ordinary field descriptions
cannot model unions, packed records, bitfields or custom compiler alignment.
Matching storage layout also does not establish C++ ownership, a language's
runtime contract, or the calling convention of an arbitrary export.

The [native comparison test](../native_layout_test.go) compiles
[C layout probes](../testdata/layout.c) and executes them from a raw object,
archive and OS library. It checks signed/unsigned integers, float/double, Boolean,
pointers, padded/nested records, record arrays, matrices and pointer arrays.
Existing Go/llgo native suites execute these tests, including Go Linux/Windows
386 and independent Go execution on Windows ARM64. Additional tests cover
unavailable builds, validation, concurrent independent queries, convention
selection, oversized aggregates and pointer queries with oversized pointees.

The implementation uses libffi's existing aggregate layout calculation and
initializes primitive metadata before reading it, following its
[size and alignment contract](https://github.com/libffi/libffi/blob/v3.4.6/doc/libffi.texi#L406-L463).
