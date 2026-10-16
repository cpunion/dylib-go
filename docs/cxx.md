# Generated C++ class facades

`compiler/cxx.Generate(header, classes)` renders a C header, C++17 source, and
ordered C export names from an explicit manifest. Generation runs entirely in
Go and invokes no compiler. The matching native C++ toolchain compiles the
source against the original class header. Complete OS libraries supply C++
runtime dependencies and exception registration.

The [generation example](../examples/cxx/generate.go) selects
`example::Counter`, its constructor, `add(int32_t)`, destruction, and a derived
class with a compiler-generated base view. The
[executed native fixture](../compiler/cxx/testdata/counter.hpp) supplies the class.
Write `Facade.CHeader` and pass its path and `Facade.Functions` to
`compiler/clang.Parse`. Its result supports:

- `CgoSource` for typed Go/llgo calls without libffi.
- `LLGoSource` for typed C function-pointer calls on qualified llgo targets.
- `ForHost` and `Session.Bind` for optional libffi calls.

`Facade.CXXSource` embeds its C declarations and includes the original C++ header.
Use a forward-slash include path; quoted paths with spaces are accepted. Include
directories, defines, ABI flags and runtime linkage belong to the producer's
compiler. The original header and any linked C++ library must match that compiler
and ABI. This package does not discover classes, demangle symbols, or parse C++.

## Manifest and C boundary

| Selection | Generated C entry |
| --- | --- |
| `Constructor{Symbol, Args}` | Fixed C arguments, followed by `void **output`; the slot must initially be null |
| `Method{Name, Symbol, Signature}` | `void *handle`, fixed C arguments, and a result pointer for non-void results |
| `Method.Const` | An exact const-qualified member pointer |
| `Method.Static` | An exact static function pointer; no receiver argument |
| `Base{Name, Symbol}` in `Class.Bases` | `void *handle, void **output`; a borrowed public base view |
| `Class.Delete` | A `void **handle` slot consumed and cleared before native `delete` |

All C exports return `int32_t`, use fixed cdecl on Windows 386, and have explicit
export visibility. C++ definitions are `noexcept`, with catch handlers converting
native exceptions into status codes. Export names must be distinct C identifiers. Class names can
be namespace-qualified identifiers or aliases; template specializations can be
named by an alias in the original C++ header. A manifest can contain up to 256
exports, matching the declaration generator's selection limit.

Supported values are `bool`, fixed-width signed/unsigned 8/16/32/64-bit integers,
`float`, `double`, `void *`, and void method results. The C++ method pointer must
match these exact native C types; equal byte widths alone do not make distinct
C++ overload types interchangeable. Constructors use normal overload resolution
for those argument types. `Const` and `Static` select their corresponding member
forms; the compiler rejects missing/private members or mismatched signatures.

Ordinary and virtual calls stay inside C++. `Class.Bases` selects public,
unambiguous base types. Generated code assigns the typed derived pointer to a
typed base pointer, so the native compiler supplies every address adjustment,
including secondary bases and shared virtual bases in a diamond. This implicit
conversion rejects downcasts, unrelated types, private bases and ambiguous
inheritance. No C++ standard-library traits or runtime discovery are required.
A native factory can also supply an already adjusted `void *` base view.
Virtual members dispatch through native member pointers; Go performs no base
offset, vtable or member-pointer arithmetic.

Base exports require a non-null output slot and clear it before checking the
receiver. A null receiver returns `InvalidArgument`. Use separate output storage
so a borrowed view does not overwrite the owning handle. The receiver must
already have the selected native C++ type; exports do not perform RTTI checks.
A view neither allocates nor retains an object and expires when its owner is
destroyed. Destroy the original owner once; do not independently delete its
borrowed views. Deleting through a base instead requires a separate native
ownership contract, including an appropriate virtual destructor.

## Status and ownership

| Status | Meaning |
| --- | --- |
| `cxx.Success` / `DYLIB_CXX_SUCCESS` (0) | Operation completed |
| `cxx.InvalidArgument` / `DYLIB_CXX_INVALID_ARGUMENT` (1) | Null receiver/output slot or an already occupied constructor output slot |
| `cxx.Exception` / `DYLIB_CXX_EXCEPTION` (2) | A native C++ exception was caught, or a custom allocation returned null |

Non-void method outputs are zeroed before checking the receiver or invoking C++.
A failed constructor leaves its initially empty slot empty. An occupied slot
is preserved so a second construction cannot overwrite an owned reference.
Delete clears its slot before invoking the destructor, including a destructor
that throws; deleting an empty slot is inert. A null slot address is invalid.
These rules do not roll back mutations a native method performed before throwing.
Exceptions from `noexcept` violations still terminate under the C++ contract.

Objects and returned native addresses remain caller-owned. Pointer results are
borrowed unless the original API supplies an ownership contract. Retain code
leases for the entire object lifetime, destroy every owned object before releasing
its provider, and stop/join any asynchronous native users before cleanup. A
`Registration` can couple code, output storage and callback leases with an
explicit destruction operation. `abi.NativeValue` owns a handle slot's bytes,
not the C++ object whose address it stores. See [registration ownership](registrations.md).

## Coverage and remaining work

Go and llgo CI compile C declarations and C++ source for all eight qualified
targets. Native jobs execute unoptimized/optimized libraries: a freshly generated
cgo caller runs without libffi, llgo also executes its direct generated caller,
and optional dynamic calls cover scalar widths,
const overloads, void results, native addresses, generated secondary/virtual base
views, composed diamond conversions, virtual dispatch,
constructor/method exceptions, throwing destruction, cleared outputs and nulls.
Linux 386 requires C++ multilib development files in addition to i386 libffi.
`DYLIB_CXX` selects the Clang-compatible C++ test producer executable.

Reference/value class parameters or results, arbitrary typed pointers, managed
C++ callbacks, ref-qualified/volatile members, variadics, inheritance discovery,
downcasts/crosscasts and C++ header discovery remain outside this manifest. Raw C++
runtime/exception loading is unqualified; use complete libraries for these
facades. The generator complements llcppg and ABIBridge while keeping C++ ABI
details in their native compiler.

References: [C++ member-pointer calls](https://eel.is/c++draft/expr.mptr.oper),
[exception handlers and noexcept](https://eel.is/c++draft/except.handle), and
[Windows exports](https://learn.microsoft.com/en-us/cpp/cpp/dllexport-dllimport).
