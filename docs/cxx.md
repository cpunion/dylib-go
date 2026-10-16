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
| `ObjectArgs` on a constructor/method | Pointer transports converted to selected native class pointers, references or copied values |
| `Method.ObjectResult` | A pointer transport carrying a borrowed pointer/reference, or a newly allocated owned value |
| `FunctionArgs` on a constructor/method | Pointer transports converted to exact fixed native function pointers |
| `Method.FunctionResult` | A pointer transport carrying a borrowed native function pointer |
| `Base{Name, Symbol}` in `Class.Bases` | `void *handle, void **output`; a borrowed public base view |
| Optional `Class.Delete` | A `void **handle` slot consumed and cleared before native `delete`; omit for borrowed/static-only selections |

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
| `cxx.InvalidArgument` / `DYLIB_CXX_INVALID_ARGUMENT` (1) | Null receiver/output slot or an already occupied constructor/value-result output slot |
| `cxx.Exception` / `DYLIB_CXX_EXCEPTION` (2) | A native C++ exception was caught, or a custom allocation returned null |

Non-void method outputs are zeroed before checking the receiver or invoking C++.
A failed constructor leaves its initially empty slot empty. An occupied slot
is preserved so a second construction cannot overwrite an owned reference.
Delete clears its slot before invoking the destructor, including a destructor
that throws; deleting an empty slot is inert. A null slot address is invalid.
These rules do not roll back mutations a native method performed before throwing.
Exceptions from `noexcept` violations still terminate under the C++ contract.

## Class arguments and results

The executable [object manifest](../examples/cxx/objects.go) uses `ObjectArg` to
select a zero-based `abi.Pointer` argument and its `ObjectType{Name, Kind, Const}`.
`ObjectPointer` accepts null. `ObjectReference` requires a non-null object address;
`Const` selects a const pointer/reference in the exact native signature.
`ObjectValue` copies the native object inside C++, with no Go layout or byte copy.
Constructors apply normal overload resolution; a value selection supplies an
explicit native temporary. Methods still use an exact member/function pointer.

`ObjectResult` requires `abi.Pointer` as the C result transport. Pointer/reference
results borrow the native API's lifetime; reference addresses bypass overloaded
`operator&`. Failed borrowed results clear the output slot. Native const results
retain an exact const signature, but the opaque Go address cannot enforce read-only
access. Receivers and argument addresses must already have their selected native
type; the facade does not perform RTTI checks or infer base offsets.

A value result allocates a separate native owner. Its output slot must initially
be null; occupied slots are preserved and return `InvalidArgument`. Allocation,
copy and method exceptions return `Exception` without publishing an owner.
Value arguments/results require accessible `noexcept` destructors: a throwing
temporary destructor could otherwise invalidate publication and cleanup. Copy
constructors may throw. C++17 prvalue results also support noncopyable/nonmovable
classes through guaranteed copy elision. Destroy each returned owner using its
native type before releasing the provider; an address slot does not own its object.

Delete exports are optional, allowing static-only classes with private destructors
and borrowed receivers. Omitting an export does not infer an alternative cleanup
operation for constructor/value results. Supply that native ownership contract
explicitly. A manifest must select at least one C export.

Objects and returned native addresses remain caller-owned. Pointer results are
borrowed unless the original API supplies an ownership contract. Retain code
leases for the entire object lifetime, destroy every owned object before releasing
its provider, and stop/join any asynchronous native users before cleanup. A
`Registration` can couple code, output storage and callback leases with an
explicit destruction operation. `abi.NativeValue` owns a handle slot's bytes,
not the C++ object whose address it stores. See [registration ownership](registrations.md).

## Native function pointers and Go callbacks

The executable [function manifest](../examples/cxx/functions.go) uses
`FunctionArg{Index, Signature}` on constructors/methods and
`Method.FunctionResult` to select exact native function-pointer types. Each
selected slot uses `abi.Pointer` as its C transport. Function/object selections
cannot overlap, and an output cannot select both result forms. Signatures accept
fixed scalar/opaque-address default/cdecl arguments and results, including void
results; the native compiler rejects mismatched callback prototypes. Null
function pointers pass through, so their validity follows the original C++ API.
Failed borrowed function results clear the output slot.

Transport uses function/object pointer conversions supported by the qualified
native compilers and targets. Returned functions borrow their provider's lifetime;
keep its symbol/function lease until the final call. A Go function value is not a
native entry: use a generated C entry or `abi.NewCallback` with the selected
signature. Dynamic Go callbacks require the optional libffi backend. The generated
cgo/direct llgo transport itself needs no libffi when using existing native entries.

Retained callbacks need callback and provider leases for their entire native
lifetime. `Registration` can couple those leases, handle storage, and native
unregister/join or destruction. The [executed dynamic test](../compiler/cxx/function_dynamic_test.go)
constructs a C++ object retaining a captured Go callback, reenters its C++ method,
and unregisters/destroys it while session, storage, and callback close are waiting.
Callbacks return Go errors through `Callback.Err` with a zero native return;
these are distinct from caught C++ exceptions. Do not unwind a Go panic or C++
exception across a C callback entry. This does not construct `std::function`,
capture a C++ lambda, or infer native callback ownership or threading rules.

## Coverage and remaining work

Go and llgo CI compile C declarations and C++ source for all eight qualified
targets. Native jobs execute unoptimized/optimized libraries: a freshly generated
cgo caller runs without libffi, llgo also executes its direct generated caller,
and optional dynamic calls cover scalar widths,
const overloads, void results, native addresses, generated secondary/virtual base
views, composed diamond conversions, class copies/references/pointers, noncopyable
value results, copy exceptions, overloaded `operator&`, private-destructor static
classes, native function factories/arguments/returns, retained captured callbacks,
nested C++/Go reentry during retirement, void/native-address/mixed-width callback
signatures, virtual dispatch,
constructor/method exceptions, throwing destruction, cleared outputs and nulls.
Linux 386 requires C++ multilib development files in addition to i386 libffi.
`DYLIB_CXX` selects the Clang-compatible C++ test producer executable.

Arbitrary non-class typed pointers, C++ lambda/`std::function` adapters,
ref-qualified/volatile members, variadics, inheritance discovery,
downcasts/crosscasts and C++ header discovery remain outside this manifest. Raw C++
runtime/exception loading is unqualified; use complete libraries for these
facades. The generator complements llcppg and ABIBridge while keeping C++ ABI
details in their native compiler.

References: [C++ member-pointer calls](https://eel.is/c++draft/expr.mptr.oper),
[exception handlers and noexcept](https://eel.is/c++draft/except.handle), and
[Windows exports](https://learn.microsoft.com/en-us/cpp/cpp/dllexport-dllimport).
