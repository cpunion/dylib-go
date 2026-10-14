# C declarations and generated dynamic bindings

`compiler/clang.Parse(ctx, header, options)` uses an optional external Clang to
preprocess a C17 header, evaluate primitive/record layout probes, and extract
explicitly requested function declarations and their type probes. Go decodes the
AST and renders source; no libclang library or compiler dependency is added to the
core loader.

Set `Options.Functions` to the C names to inspect. `Options.Compiler` selects the
executable; `Options.Target` defaults to the qualified native host triple.
`Options.Flags` supplies matching include directories, defines and preprocessing
options. The explicit target and C17 mode take precedence over flags. Headers
and included files are read by Clang at their original paths and must remain
stable during parsing. This preserves the original relative include search.

The result is a `Header` with target metadata and ordered `Declaration` entries.
Each entry contains the source `Name`, loader object `Symbol`, and `abi.Signature`.
`Lookup` clones signature storage. Prefer `ForHost` before binding: it checks OS,
CPU and pointer width, and checks every used record's compiler size, alignment and
member offsets against `abi.LayoutOf` before returning the declaration. Record
validation needs the optional libffi backend; metadata inspection and source
generation do not. Use `Session.Bind` with its
symbol and signature. The normal image loading and symbol lifetime rules apply.

`Header.GoSource(packageName, variableName)` emits a target-qualified Go `Header`
variable with ordinary imports. It uses no `go:linkname` directives. Regenerate
for each supported target; an incompatible generated header fails `ForHost`.
The [generation utility](../examples/declgen/main.go) exercises this library API.
The [tested library example](../examples/declarations/main.go) consumes generated
declarations and passes a generated struct callback to C through `Session.Bind`.
Run
`examples/run.sh <go|llgo> declarations <library>`.

## Types and calling conventions

| Declaration | Generated behavior |
| --- | --- |
| `void`, `_Bool`, `float`, `double` | Supported scalar results/arguments; `void` is a result only |
| Signed/unsigned C integers | Fixed ABI widths from evaluated compiler size probes |
| Plain `char` | Compiler-confirmed signedness |
| Named and typedef enums | Compiler-confirmed compatible signed/unsigned 8/16/32/64-bit integers or `_Bool`; packed/short enums and Clang fixed-underlying-type extensions are included |
| Typedefs and ordinary qualifiers | Desugared/resolved, including integer and pointer aliases |
| Scalar/enum/native-pointer typedef `aligned(N)` or omitted `aligned` | Scalar calls retain their ordinary ABI; record fields and scalar/native-address array leaves use compiler-validated C bridges, preserving raised/lowered alignment and alias chains |
| Fixed-array typedef `aligned(N)` or omitted `aligned` | `CgoSource` preserves each array alias layer independently, including scalar/enum/record/native-address elements, nested aliases, lowered/raised alignment and packing combinations |
| Named bitfields with an aligned scalar/enum typedef | Compiler-validated C bridges preserve effective allocation boundaries, record alignment and argument lowering; unnamed/zero-width aligned typedefs are rejected |
| Object/opaque pointers | Native addresses; no automatic pointee allocation, copying or ownership |
| Ordinary structs, including anonymous typedef records | Arguments/results; compiler-evaluated layout checked against the native backend |
| Structs with `#pragma pack` | `CgoSource` for Go/llgo; caps 1/2/4/8/16, over-aligned ordinary members/bitfield units/zero-width boundaries, nested records/arrays and native addresses/function-pointer fields |
| Global Clang `-fpack-struct` defaults | `CgoSource` for Go/llgo; omitted argument or caps 1/2/4/8/16, compiler-effective options, local pragma override/pop, nested alignment/arrays and qualified bitfields; transports keep ordinary layout |
| Explicit `__attribute__((packed))` structs | `CgoSource` for Go/llgo; logical fields are transported through ordinary records and converted inside C; dynamic and direct llgo bindings reject them |
| Explicit `__attribute__((packed))` non-bitfield members | `CgoSource` for Go/llgo; retain natural alignment of other members and combine packing with explicit member alignment; includes fixed arrays, nested records and native addresses |
| Named integral/Boolean bitfields, including explicit packed records | `CgoSource` for Go/llgo; compiler byte masks and layout are validated before binding; public fields remain ordinary scalars |
| Unnamed/zero-width bitfields | `CgoSource` preserves storage boundaries without adding public Go value fields; leading/trailing padding, explicit packing/alignment and packed combinations are supported |
| Compiler-evaluated record/member alignment | `CgoSource` preserves GNU `aligned(N)` and omitted `aligned`, field `_Alignas(N)`/`_Alignas(type)` and MS `__declspec(align(N))`; packed/bitfield combinations and nested arrays are supported |
| Fixed array members | Scalars, pointers, records and nested arrays; positive evaluated lengths |
| Function parameters/results pointing to a complete ordinary struct | Typed single-struct pointees; `abi.AddressOf` supplies a temporary copy, with mutation/return identity checks |
| Function pointers at parameter/result boundaries | Typedefs, anonymous pointers and decayed function parameters; retain their exact prototype/convention independently of the outer pointer |
| Variadic prototypes | Fixed prefix; `Declaration.WithTail` adds a concrete scalar/pointer tail; `abi.Prepare` applies promotions |
| C cdecl | All eight qualified native targets |
| stdcall / fastcall | Windows 386; retain compiler symbol decoration |
| Higher-order and multiply indirect function pointers | Nested argument/result prototypes and 1–8 pointer layers; caller owns code and pointee lifetimes |
| Function-pointer record fields and fixed arrays | Opaque native address storage with separate field-indexed prototypes; supports typedef/direct pointers, multidimensional arrays, nested record arrays and self-referencing callback signatures |
| Unions, other attributed records/fields/typedefs, flexible/zero-length arrays, inaccessible anonymous enums, incompatible/overaligned enum integers, atomics, long double, extended integers or other conventions | Rejected by this generator; manual adapters remain independent |
| Static/inline functions and old-style declarations without prototypes | Rejected; provide an exported C facade/prototype |

Enums are resolved with compiler `sizeof`, `_Alignof`, signedness and compatible
integer probes. Enumerator constant types alone do not identify a packed or
short enum's storage. The same header can produce different signatures: MSVC
ordinary enums use `int` even where GNU/Apple targets choose an unsigned or short
integer. Match `Options.Flags` to the producer, including `-fshort-enums`.
Fixed underlying types are accepted when Clang supports their syntax in C17 mode.

Enum values use the existing numeric/Boolean Go types and `abi.Value`
constructors. Generated C bridges and direct llgo entries use the compatible
integer ABI, including function pointers and record members. Enum constants and
symbolic enum argument names are not generated or parsed. Enum pointers remain
opaque native addresses. Incomplete enums require a fixed underlying type for
by-value calls; anonymous enums need an accessible typedef. Extra enum alignment
or a compatible integer outside the supported widths is rejected.

`Header.Records` saves the C type spelling, `abi.TypeDesc` and compiler layout for
each used record. `LookupRecord(name)` clones that metadata for constructing values
with `abi.StructValue`/`abi.ArrayValue`. Clang declaration IDs associate anonymous
records with their typedef names. A second compiler invocation evaluates
`sizeof`, `_Alignof` and `__builtin_offsetof` with the same header/target/flags.
Generated source retains all layout checks. Compiler default packing is retained
in `Record.MaxFieldAlignment` and requires a compiled C adapter; `ForHost` rejects
it before a dynamic call.

Pointers within record members remain opaque addresses, including recursive
record links. Incomplete record pointers also remain opaque. Complete struct
pointers at function boundaries describe one pointee; array bounds, retained
storage, allocation and ownership are not inferred from a pointer prototype.
Use native addresses or an explicit manual descriptor for other pointee storage.
C array parameters decay to pointers; bare arrays are not passed by value.

`Declaration.FunctionPointers` stores each function-pointer signature with its
zero-based parameter `Position`; `-1` identifies a function-pointer result.
`LookupFunctionPointer(position)` returns a detached snapshot. Its own
`LookupFunctionPointer` method inspects nested parameter/result prototypes.
`Indirection` counts pointer layers; zero is the backward-compatible one-layer
default. The outer signature
uses `abi.Pointer`, so pass a leased `abi.NewCallback` address to C. `ForHost`
checks records used by both the outer call and each function-pointer signature.
The generated example derives its struct callback signature from these metadata.

Clang `__typeof__` probes expose `FunctionProtoType` nodes in the same AST
invocation. Their parameter/result types and effective calling convention avoid
parsing nested C declarator text. Function-pointer signatures support ordinary
records and typed struct pointers, along with the existing primitive types.
Callbacks can accept/return other native function pointers and use up to eight
prototype levels and eight pointer layers.

`Record.FunctionPointers` uses `Position` for the zero-based field index.
`Record.LookupFunctionPointer(fieldName)` returns a cloned prototype, including
its nested callbacks. For fixed array fields this is the common element
prototype; dimensions remain in `Record.Description`. Clang's evaluated
`ConstantArrayType` nodes supply the extents, including multidimensional arrays,
arrays of higher-order entries and arrays of indirect entry slots.
`Header.ForHostRecord(name)` checks the target and native
layouts before returning the record. `ForHost` also validates records used only
by a field callback prototype. Field type nodes come from an extra batched
Clang `__typeof__(((Record *)0)->field)` probe when ordinary spelling resolution
cannot describe a field; C declarator text is not parsed. Physical record
descriptions are completed before callback prototypes, so a callback accepting
its enclosing record does not create a recursive metadata graph.

Generated cgo and llgo record fields use `unsafe.Pointer` or fixed Go arrays of
that type. Prototype metadata
does not make an address callable as a Go function value or own its lifetime.
Pass a native entry with its image lease, or create a captured Go entry with
`abi.NewCallback(recordPrototype.Signature, handler)`. Hold a callback lease while
C retains the field; `CallbackLease.Pointer()` supplies its native address to
typed cgo/llgo record fields. `Registration` can couple the image and callback leases
with an explicit operation that clears native storage before releasing them.
The [record example](../examples/recordcallbacks/call.go) and its tests exercise
by-value arguments/results, nested record arrays, native struct pointers,
field replacement, null entries, direct noncapturing llgo entries and retained
Go captures. Runtime tests use objects, archives and shared libraries at Clang
`-O0` and `-O2`; cgo runs on eight targets and direct llgo on six. Tests include
multidimensional callback arrays, element order, replacement, null entries and
registrations retaining multiple Go captures until native storage is cleared.
Flexible/zero-length callback arrays and entries without prototypes are rejected.

For native function-pointer results, retain the image while calling the returned
address, for example inside the factory symbol's `Symbol.WithAddress` lease.
`FunctionPointer.WithTail` expands a variadic native pointer's fixed prefix;
`abi.NewCallback` still rejects variadic callback entries. Callback leases,
registration removal, borrowed argument storage and returned native addresses
follow the existing [callback lifetime contract](callbacks.md). No registration
or address ownership is inferred from a C prototype.

Headers describe declarations rather than proving actual exports. The loaded
binary must match the header, compiler ABI and flags. DLL export aliases can
differ from object symbol decoration; choose the actual exported name explicitly.
No language runtime, C++ ownership or exception adaptation is inferred.

## Typed cgo bridges for Go and llgo

`Header.CgoSource(packageName, bindingType)` emits typed methods with static C
bridges in an `import "C"` preamble. It supports cdecl scalars, ordinary/explicitly packed or aligned and bitfield record
values and opaque native pointer arguments/results on all eight native Go targets and six qualified llgo
targets, without the optional libffi backend. Fixed Windows 386 stdcall/fastcall
exports and function-pointer prototypes are supported with Go. A C toolchain is needed when building
the generated package; neither Clang nor the original header is required at runtime.

For a variadic export, replace its declaration with the result of
`Declaration.WithTail(types...)` before generation. The method has typed parameters
for that concrete shape, while the C bridge calls through the true variadic
prototype. C applies integer and floating-point default argument promotions;
fixed parameters retain their declared types. An unexpanded prefix generates a
zero-tail call. It is the caller's responsibility to match any native format/count
contract. Generate a separate binding type for a different tail shape.

Function-pointer parameters/results use `unsafe.Pointer` on the Go boundary.
The bridge restores the saved C prototype, including variadic native entries,
before passing the address to C. It does not generate Go callbacks or acquire
ownership of a returned address. Keep producer images and callback registrations
leased throughout their later use. Borrowed pointers must refer to native storage
or obey the host compiler's C pointer rules; C must not retain borrowed Go storage.

Record fields support nested records, fixed arrays and opaque native addresses.
Generated public types use `<bindingType>Record<index>` and exported field names.
Conversions copy fields recursively into separate C values; they do not cast Go
struct storage or depend on Go padding/alignment. The C compiler determines
aggregate argument/return classification, including hidden return storage.
Constructors check each C record's `sizeof`, `_Alignof` and `offsetof` against the
saved Clang layout. Packing flags or otherwise incompatible layouts without explicit record attributes fail before symbol
resolution. Complete struct pointees require native storage. They are represented
by `unsafe.Pointer` on the Go boundary,
and the C bridge restores their saved record-pointer prototype. Returned addresses
remain caller-owned/borrowed as required by the producer. The generator does not
cast pointers to public Go records or infer ownership, allocations or pointer
graphs. Keep the native storage and image alive throughout address use.

Use producer-owned native storage, a caller-defined allocator, or
`abi.NewNativeValue(record.Description, initial)` after comparing its layout
with the saved Clang layout. A native value lease's `Pointer()` supplies an
`unsafe.Pointer` without an integer-to-pointer conversion. That allocation path
requires libffi; generated pointer methods themselves do not. Other typed
pointees and typed pointer fields in manually assembled record metadata remain
unsupported; record fields use opaque addresses.

### Aligned scalar, pointer and array typedefs

`typedef int Low __attribute__((aligned(1)))` keeps the integer size while
lowering its alignment. A member `aligned(1)` cannot reproduce this change.
`CgoSource` preserves scalar, enum and native-address aliases, including numeric
or omitted alignment, inherited aliases and later alignment overrides. Fixed
scalar/native-address arrays keep the alias at their leaves. Record packing,
pragma caps and explicit member alignment remain separate metadata.

Whole-array aliases are also supported: `typedef int Pair[2]
__attribute__((aligned(1)))` lowers the array's alignment without changing its
size or its integer elements. Nested aliases retain independent requests at
each layer, including scalar, enum, complete-record and native-address elements.
For example, an aligned scalar can form an aligned row, which can then form a
matrix with a lower alignment. Aliases wrapping that matrix preserve all three
requests. Clang rejects invalid scalar strides; `CgoSource` also rejects an
aligned array alias nested in another array if its size is not a multiple of
the requested alignment. Clang accepts that form while GCC rejects it.

`Record.FieldTypeAlignments` contains the requested alias alignment and measured
member storage alignment. `ArrayDepth` is zero at the scalar/native-address
leaf, one at the innermost fixed array, and increases towards the outermost
array. Entries are ordered by member position and then depth; the storage
alignment applies to the whole member and agrees across its entries. A zero
depth preserves compatibility with earlier scalar typedef metadata.
Probes compare expression alignment with alignment in
a standalone member wrapper, then reconcile it with the actual record base and
offset. This matters on Windows: MSVC/MinGW may ignore lowered typedef alignment
for scalar or pointer members while preserving it for arrays or enums. Windows
C facades preserve the observed placement before the usual size/alignment/offset
checks. GNU AMD64 facades use packed natural scalar members with the measured
alignment: this preserves canonical SysV memory/register classes across the C compilers.
`Parse` probes the effective calling ABI, including the supplied flags. Clang 18
has incompatible lowering for these aliases (including truncated floating
records), so GNU AMD64 records require Clang 19-compatible lowering. Scalar
boundary aliases and other targets do not require this probe. Public Go fields and transport records
retain ordinary layout.

The [typedef example](../examples/typedefalignment/call.go) executes small/large,
floating, enum and multidimensional-array records, packed/pragma combinations,
numeric/omitted alignment, native pointer/entry fields, factories, pointer
mutation and concrete variadic calls. Go and llgo CI execute object, archive and
OS-library inputs at `-O0` and `-O2`, without requiring libffi. The compiler tests
also check all eight target layouts and MSVC-producer/MinGW-bridge compatibility.
Whole-array cases additionally cover three-dimensional mixed-alignment aliases,
record/native-address/entry arrays, lowered floating arrays, array parameter
decay to scalar pointers, variadic prefixes and provider-owned pointer mutation.

Scalar or pointer typedef alignment alone does not change the scalar calling
interface, so those boundary signatures also support `Session.Bind` and direct
llgo adapters. Attributed record values still require C bridges. Aligned
record typedefs and aligned unnamed typedef bitfields remain rejected; they
need additional type identity and compiler ABI qualification. Unsupported
attributes in an alias chain are rejected before generation. See GCC's
[typedef alignment rules](https://gcc.gnu.org/onlinedocs/gcc-15.2.0/gcc/Common-Type-Attributes.html).

### Aligned typedef bitfields

Named integral, Boolean and enum bitfields accept aligned base typedefs,
including numeric/omitted requests, inherited aliases and alignment overrides.
`Bitfield.TypeAlignment` retains the request; `StorageAlignment` retains a
compiler-qualified allocation boundary, or zero for a shared/ignored request.
These values are separate from explicit member alignment and packing.

A short field can stay in an earlier allocation unit while its typedef raises
the record alignment. Wider fields can start a new aligned unit. GNU C facades
preserve both observations using canonical bitfield types, effective member
attributes and the measured record alignment. MSVC probes reconstruct the
original base typedef to observe active allocation-unit starts; MinGW bridges
preserve their alignment even inside packed units. Generated constructors
validate sizes, alignments, offsets and byte masks before binding symbols.
MinGW facades also restore the measured alignment of packed records because GCC
can suppress an aligned unit's contribution to record alignment. MSVC retains
its natural alignment so that an enclosing packing pragma can cap it.

On Linux AArch64, natural 16-byte alignment can require an even general-register
pair for a small record argument. Explicit record alignment alone does not
reproduce this rule. `Record.ArgumentAlignment` records Clang's argument lowering;
a private zero-length naturally aligned C member preserves that rule without
adding a public field or changing storage. Other targets omit this marker.

The [typedef bitfield example](../examples/typedefbitfields/call.go) executes
short/wide fields, shared units, signed/Boolean/enum values, explicit member and
record packing, pragma caps, nested arrays and floating records. Go and llgo
tests load objects, archives and OS libraries at `-O0`/`-O2`, exercising fixed
and variadic calls, preceding scalar/native-entry arguments, factories and
producer-owned pointers with mutation/null checks. Compiler tests compare all
eight target layouts, three Windows GNU producers, MSVC/MinGW bridges and
LLVM argument/result shapes; GCC independently checks native layout and masks.
Aligned unnamed/zero-width base typedefs remain rejected.

## Explicitly packed C records

`Record.PackedFields` preserves the logical positions of explicitly packed
named members, including bitfields. It reduces those members' alignment while leaving the
other members' natural alignment intact. C bridges reproduce field attributes,
including `packed` combined with numeric `aligned(N)`, and validate the resulting
size, alignment and every offset. Public Go fields remain ordinary values.
Nested arrays and pointer/factory/variadic calls use the same C transport path.
Dynamic and direct llgo adapters reject these attributed records. Use native
producer storage for their pointers; `abi.NewNativeValue` describes logical
storage and does not reproduce member packing. Packing through typedefs remains unqualified.

For member-packed MS-layout bitfields, MinGW ignores the member attribute.
The generated C bridge groups each affected allocation unit in an anonymous
record with a scoped packing pragma and restores any explicit unit alignment.
Neighboring ordinary fields retain their natural alignment. Size, alignment,
ordinary offsets, and every bit mask must match the producer before binding.
Unnamed fields remain in declaration order inside their allocation units.
A zero-width boundary immediately after a wrapped unit uses a zero-length C
array on MinGW to retain natural alignment without allocating bytes; a static
assert checks its offset against the producer. Leading and consecutive zero-width
fields keep their original semantics. These combinations are supported in both
otherwise unpacked and record-wide packed records. `BitfieldPadding.Packed`
preserves explicit packing on unnamed declarations, including zero-width fields.
The bridge also wraps allocation units whose first member is unnamed and packed;
packed zero-width boundaries retain byte alignment through the zero-length array.
Explicit unnamed alignment is preserved as described below.

`Record.Packed` preserves Clang's explicit `PackedAttr`. `Record.Description`
remains a logical field model, not a descriptor of packed native memory. Generated
C declarations reproduce `__attribute__((packed))`; their layout probes must
match the saved size, alignment and every offset before symbols are resolved.

For packed values and ordinary parents containing them, the generator also emits
ordinary transport structs. Go copies its public fields to/from those transports.
C copies transports to/from the actual native type and applies its own argument
and return ABI. Nested records, multidimensional arrays, Boolean/narrow fields,
native address fields, function-pointer prototypes and concrete variadic calls
retain this separation. Neither Go nor llgo passes a packed C record directly.
This path needs a C compiler at build time and does not need libffi.

`ForHost`, `ForHostRecord` and direct `LLGoSource` reject packed records, including
reachable nested records and callback prototypes. Use producer-owned native
storage for their pointers; `abi.NewNativeValue` cannot allocate packed storage
from the logical description. Native function pointers using packed values need
a matching compiled adapter; `abi.NewCallback` does not supply this ABI.

Unions and unqualified packing/alignment forms are not inferred by this adapter.
When two records have identical
logical fields but different native layouts, typed generation rejects the
ambiguity rather than selecting a layout from metadata order. Generate separate
bindings for those declarations.

The [packed example](../examples/packedrecords/call.go) and its native tests run
objects, archives and shared libraries at `-O0` and `-O2` through the existing
setup-go/setup-llgo jobs. They verify packed values/returns, ordinary parents with
packed multidimensional arrays, signed boundaries, Boolean and pointer fields,
factories/consumers, native struct pointers, variadic promotions, stale generated
source, target guards and session retirement.

### Pragma packing

`Record.MaxFieldAlignment` preserves the compiler's numeric `#pragma pack`
member-alignment cap; zero means absent. Caps of 1, 2, 4, 8 and 16 bytes are
qualified for ordinary records and bitfields. Clang evaluates nested push/pop
directives, including directives in included headers. This is separate from the
`packed` attribute and from the record's effective alignment.

Clang JSON identifies `MaxFieldAlignmentAttr` without its numeric argument.
A bounded supplemental text AST query uses synthetic typedefs to link each
accessible record spelling to its compiler-reported cap. IDs are matched only
within that invocation. Missing, duplicate, ambiguous or invalid probes fail
parsing; source pragma tokens and layout guesses are not used.

Generated C declarations use a scoped packing pragma, restored before emitting
transport records. Ordinary MSVC records have a measured-member MinGW facade
as described below. Public Go fields carry logical values. Native layout checks
run before binding, and dynamic/direct llgo adapters reject these records.
Arrays, nested ordinary/bitfield records, native struct addresses, function-pointer
fields/factories and concrete variadic prefixes/results use the same C bridge.
Record-level explicit alignment can increase alignment while member offsets
remain packed. Use producer-owned storage for pointers.

Named, unnamed and zero-width bitfields preserve their declaration order and
compiler-confirmed byte masks/boundaries. Prefix and unnamed-field probes use
the same packing cap as the original record. On Windows, MinGW zero-width
fields can bypass that cap; the MSVC bridge restores active boundaries with
zero-length native members while retaining ignored leading/consecutive fields.
Record-wide and member-level packed attributes can be combined with pragma
packing. These changes remain inside C and do not add public Go padding fields.

Ordinary members and nested ordinary values may have explicit alignment greater
than the packing cap. MSVC can override that cap while GNU producers limit the
request. `Record.MemberAlignments` retains each ordinary pragma member's measured
storage alignment, queried alongside its size/offset layout. The expression
alignment is bounded by the record's actual alignment and member offset: a GNU
packed record can otherwise report a source request it does not preserve.
These independently cloned/generated values are distinct from `FieldAlignments`.
Regenerate older pragma bindings to include the measured member metadata.

For an MSVC producer, the MinGW facade retains every native field type/array count
and applies measured packed/aligned member attributes outside the outer pragma.
The MSVC compiler retains the native pragma declaration. Avoiding synthetic byte
fields preserves homogeneous aggregate classification. Existing MSVC i386
compiler-observed indirect argument lowering still applies. Native size,
alignment and offset checks remain mandatory before resolving symbols.

Pragma bitfield records also retain measured storage alignment. Named MS fields
use `MemberAlignments` at allocation-unit starts, and zero inside a shared unit.
`BitfieldPadding.StorageAlignment` identifies active unnamed unit starts and
zero-width boundaries; ignored/shared declarations use zero. Small compiler
probes preserve packing and source requests while excluding unrelated root
alignment. This avoids interpreting an ignored request as actual storage.

The MinGW facade emits complete byte-aligned anonymous allocation units and
applies measured alignment to the anonymous member, preserving unit size.
Ordinary members keep their native types and measured alignment; active zero
boundaries use zero-length native arrays, and ignored declarations retain their
type without their ineffective request. Leading padding-only units use an empty
C initializer. Original MSVC/GNU producers keep their source declarations.
Regenerate older pragma bitfield bindings to retain this additional metadata.

For Linux/macOS GNU layouts, GCC can retain requests that Clang ignores and
raise record alignment for zero widths. Generated bridges drop ignored nonzero
requests. `BitfieldPadding.Gap` retains the compiler-observed bytes before an
over-cap zero boundary; unnamed padding recreates those bytes. Record alignment remains the
producer's measured value. Cross-target layout/mask and LLVM call-shape checks,
plus independent GCC bridge tests and native CI calls, qualify this facade.
On GNU AMD64, small trailing gaps use implicit body padding and a lowered
typedef alignment: GCC would otherwise count unnamed padding in argument/result
registers. Native tests check integer and one/two/three-float records, including
register positions and promotions in variadic calls.
Windows MSVC i386 single-float aggregates return in integer registers, while
MinGW normally uses x87. The C facade uses a private overlapping integer member
to preserve MSVC's return registers and keeps the public value in an ordinary
transport record. Native tests cover `float`, `double`, a single-element array
and a nested record; compiler checks also cover explicit record/member packing.
Small naturally aligned GNU AMD64 records use anonymous aligned prefix records
for interior zero-width gaps. Each prefix rounds up to the measured boundary;
the outer pragma preserves the native record alignment and public field offsets.
Multiple boundaries nest these prefixes, and a trailing gap can still use the
lowered-alignment typedef facade. Neither form adds an integer padding value to
the argument/result registers. Native calls cover floating fields, named
bitfields, signed bytes, nested records, arrays and variadic register positions.
Interior gaps mixed with original unnamed nonzero bitfields remain unqualified
and are rejected before generation.

GNU AMD64 also requires care with nonzero trailing unnamed bitfields. Clang can
ignore their padding when assigning argument/result registers, while GCC counts
it as an integer value. Matching sizes and offsets alone cannot detect this.
For small records without named bitfields or earlier nonzero unnamed fields,
`CgoSource` replaces trailing unnamed values with implicit tail padding when the
measured size is a power of two, up to 16 bytes. The private body rounds up to
that size and its typedef restores the measured native alignment. This preserves
both register classes and stack allocation when registers are exhausted.
Other small trailing unnamed layouts in this subset require another adapter and
are rejected before generation, including a 12-byte pair of floats with padding.
Other targets retain their native declarations.

The [tail padding example](../examples/tailpadding/call.go) runs float/double,
integer and array values, nested arrays, native factories and pointer mutation.
Object/archive/library calls at `-O0`/`-O2` exercise exhausted floating/integer
registers, subsequent stack arguments, variadic promotions and returns through
Go and llgo. Compiler tests independently check eight target layouts, LLVM call
shapes and GCC native sizes/alignments/offsets.

The [over-aligned bitfield example](../examples/overalignedbits/call.go) executes
small/large values, ignored shared requests, named/unnamed/zero-width boundaries,
packed combinations, floating aggregates, multidimensional nested arrays,
aligned native function-pointer fields, factories, pointer identity/mutation,
nulls and concrete variadic prefixes/results at `-O0`/`-O2` with Go and llgo.
Record typedef alignment and aligned unnamed typedef bitfields remain unqualified. The
[packed example](../examples/packedrecords/call.go) executes object/archive/library
calls at `-O0`/`-O2`, including nested packing levels, restored natural outer
layout, numeric/omitted/type-based member alignment, packed combinations, nested
aligned arrays, floating aggregates, Boolean/address fields, native
factories/pointers/function-pointer fields and variadic calls with Go and llgo.
The [bitfield example](../examples/bitfieldrecords/call.go) adds small signed
returns, active/consecutive/trailing boundaries, packed/aligned unnamed fields,
floating aggregates, nested arrays, native pointer mutation/factories and
variadic bitfield prefixes/results at both optimization levels.

### Compiler default packing

Pass the producer's global packing settings in `Options.Flags`, including Clang
[`-fpack-struct`](https://clang.llvm.org/docs/ClangCommandLineReference.html#cmdoption-clang-fpack-struct-arg).
The omitted argument selects one byte; numeric caps 1/2/4/8/16 are qualified.
Clang does not attach this language-option default to the record AST. For used
records, the parser inspects the driver's `-###` frontend command with the same
target and options, then retains the cap before validating record declarations.
This includes response files, frontend option forwarding and compiler defaults;
flag text inside a macro value is never interpreted as a compiler option.
Missing/ambiguous frontend commands and unsupported caps are rejected.
Driver output uses the existing bounded process/cancellation machinery.

Local pragma caps override the global default and `pack(pop)` restores it.
MSVC ignores pragma caps greater than the pointer width, retaining any global
default instead: `pack(8)` with a global cap of two still packs at two on Windows
i386. GNU targets honor that local cap. Compiler-observed size, alignment,
offsets, bit masks and i386 argument lowering remain authoritative.

`CgoSource` applies packing to native declarations individually. Do not pass the
global packing flag when compiling generated transports: those records and C
headers retain their ordinary host layout. Typedef attribute limits still apply.
Dynamic/direct llgo adapters reject these records;
use the compiled C bridge with Go or llgo.

The [global packing example](../examples/globalpacking/call.go) executes raw
objects, archives and OS libraries at `-O0`/`-O2`, covering signed/Boolean/native
address fields, named/zero-width bitfields, nested arrays and aligned floating
values, pragma override/pop, native factories/function-pointer fields,
pointer mutation/nulls and variadic prefixes/results.

### Explicit C alignment

`Record.Aligned` retains a numeric record alignment; zero means no explicit
request. `Record.FieldAlignments` records the logical position and alignment of
each named member, including array and bitfield members. Clang evaluates constant
alignment expressions. Omitted GNU `aligned` arguments use a compiler probe
for the target maximum alignment. Type-based `_Alignas(type)` on named ordinary
members uses a compiler probe of the member expression, including typedef,
record and array types. Its effective alignment is saved as a numeric request,
so generated bindings do not need the original alignment type. Multiple
declarations use the greatest value.
The adapter accepts powers of two up to 65536 bytes, subject to the same maximum
record size. `_Alignas(0)` has no effect.

Generated native types reproduce these requests with `aligned(N)` attributes.
Logical Go fields cross the boundary through ordinary transport records; C
creates the aligned values and applies their native argument/result ABI. This
also preserves nested array strides, packed combinations, aligned bitfields,
native function-pointer prototypes and concrete variadic calls. Constructors
compare native size, alignment, field offsets and bit masks before resolving
symbols. Explicitly aligned records and reachable nested records are rejected
by dynamic `ForHost` and direct `LLGoSource` adapters.

For MSVC i386, Clang also emits small argument-lowering probes as LLVM IR.
Go reads these probes without executing foreign code and saves
`Record.IndirectArgument` and `Record.IndirectResult`.
Required fixed arguments can use an implicit pointer
even when an ordinary record has the same storage alignment. Generated bridges
reconcile this with a MinGW host using local aligned C copies, including native
function pointers, cdecl variadic prefixes and fixed stdcall/fastcall calls.
Unnamed variadic aggregate arguments retain their by-value ABI. This follows
the [Clang MSVC i386 argument classification](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.0/clang/lib/CodeGen/Targets/X86.cpp#L762).
GCC fastcall also places an implicit result pointer differently from MSVC/Clang.
Its bridge places register arguments first, then an explicit stack result carrier
and the remaining stack arguments; the producer keeps its original ABI. Return
probes also cover ordinary records used by fastcall exports or native pointers,
including inline record arguments before the register arguments. These
MSVC i386 hidden-result records require a compiled C adapter as well.

For MinGW packed bitfields, the byte-packing pragma can also suppress member
alignment. The bridge reproduces compiler-confirmed member boundaries with
unnamed byte fields and restores record alignment. Prefix marker offsets avoid
mistaking tail padding for the boundary after a zero-width field. Native layout
and mask checks still run before binding.

Use producer-owned storage for aligned record pointers. `abi.NewNativeValue`
uses the logical description and cannot provide this attributed layout or its
stronger alignment. Generated pointer methods borrow native addresses; they do
not reinterpret Go structs. Function pointers with these values also require a
matching compiled adapter; dynamic callbacks do not supply it.

`BitfieldPadding.Aligned` retains numeric or omitted GNU alignment requests on
unnamed and zero-width bitfields. MSVC can ignore these requests inside a shared
allocation unit or on a leading/consecutive zero-width declaration. The saved
request is therefore separate from the effective native layout. For an MSVC
producer, MinGW bridges suppress ignored requests and reproduce effective
boundaries with aligned allocation units or compiler-confirmed byte gaps.
MinGW producers retain their original declarations and ABI.

Alignment on typedefs remains unsupported. Named member requests whose alignment
cannot be confirmed in the native layout remain rejected outside the qualified
pragma bitfield bridges. Other packing policies are not inferred. The [aligned example](../examples/alignedrecords/call.go)
executes object/archive/library calls at `-O0` and `-O2` on the eight Go and six
llgo native targets, including large nested arrays, floating aggregates,
native storage alignment, type-based member requests, omitted GNU arguments,
factories, variadic calls and deliberate alignment
mismatch rejection before symbol resolution. Windows 386 also runs aligned
stdcall/fastcall exports and native factories with mixed register/stack arguments.

### C bitfields and padding

`Record.Bitfields` records each field's logical position, declared width and
native byte mask. `Layout.Offsets` contains the mask's first byte. Signed and
unsigned integral base types up to 64 bits and `_Bool` are supported.
`Record.Padding` retains unnamed and zero-width declarations by logical field
position, base type, width, explicit packing/alignment and compiler-confirmed
zero-width byte boundary. Aligned nonzero padding also saves its first byte.
These declarations are omitted from public Go value fields,
including leading/trailing padding. A record needs at least one named value
field, with at most 32 named/padding declarations in total.

C forbids `offsetof` on bitfields. Parsing compiles a zero-initialized constant
for each field with all its bits set, then reads the target ELF, COFF or Mach-O
object with Go's object parsers. Cross-target parsing does not execute foreign
code. Generated C declarations restore the field widths and constructors check
the masks as well as size, alignment and offsets before resolving exports.
For aligned nonzero padding, a synthetic declaration names just that field and
emits an all-bits-set constant. Naming it can increase tail alignment, so only
the original record's byte range is read; size and alignment always come from
the original declaration. This also qualifies records without named bitfields.
On Windows, byte-aligned packed bitfields also use a scoped packing pragma:
MinGW otherwise ignores the record's packed attribute for MS-layout bitfields.
Producer objects and libraries must use the ABI selected during parsing. Tests
link the target-qualified producer object into each library and compare MSVC
and MinGW bridge sizes, alignments, ordinary offsets and bitfield masks.
For packed records with unnamed fields, MinGW also differs in zero-width
alignment. Its generated type preserves MS allocation units, uses byte-aligned zero-width
fields with compiler-confirmed gaps, and restores the saved record alignment;
other targets retain the original declarations. Prefix size assertions reject
an incompatible compiler boundary without depending on `gcc_struct` support. Named field types and prototypes remain
intact, and layout/mask checks still run before binding.

Public Go fields are ordinary scalar values. C converts them through ordinary
transport structs and applies the native bitfield ABI, including narrowing and
sign extension. Packed bitfields, nested records/arrays, native factories and
concrete variadic calls use the same path. Dynamic libffi and direct llgo
bindings reject these records. Their pointers require producer-owned native
storage; logical descriptions cannot allocate or reinterpret native bitfields.

The [bitfield example](../examples/bitfieldrecords/call.go) executes optimized and
unoptimized objects, archives and libraries with Go and llgo. Tests cover signed
boundaries, Boolean values, cross-byte 40-bit fields, unnamed/zero-width
boundaries, leading padding, numeric/omitted unnamed alignment, ignored MSVC
requests, floating fields, packed combinations, nested arrays, native
pointers/function pointers and variadic promotions. A
binding with a shifted mask verifies rejection even when size, alignment and
the field's first byte still match.

Constructors check `Target.CheckHost` before layout checks and resolving exports. Methods run their
C bridges inside `Symbol.WithAddress`, retaining the image for the call and
rejecting closed sessions. Nil/zero bindings return `dylib.ErrClosed`. Bridge
parameters/results use 32-bit integer transport for smaller integer types and
Boolean values; C restores declared widths before the native call. This avoids
depending on narrow-parameter extension across the Go/C boundary. Native pointer
typedefs carry explicit cdecl/stdcall/fastcall attributes on Windows 386,
independently for each outer/inner prototype. The static cgo bridge keeps the
ordinary host C convention. Other typed pointees and conventions
fail generation. Non-cdecl variadic calls and stdcall/fastcall on other targets
are rejected. Use dynamic
`Session.Bind` or the direct llgo adapter for those supported interfaces.

The [generation utility](../examples/declgen/main.go) accepts `-cgo -var=Bindings`
and repeated `-tail name=int8,float32` options. An empty `name=` supplies no tail.
The [library example](../examples/cgodeclarations/main.go) and tests compare fresh
Clang output with eight committed target variants and execute every scalar width,
Boolean/native pointers, zero/void calls, fixed/variadic mixed floating-point
arguments, narrow integer/float/Boolean promotions, wide values and native pointers
in a variadic tail, fixed/variadic native factories and address forwarding.
`examples/run.sh <go|llgo> cgodeclarations <library>` builds and runs it without
`-tags libffi`; the standard example suite checks that it prints 42. Native
Windows 386 CI also executes repeated stdcall/fastcall mixed scalar calls and
native factory/consumer round trips, including opposite outer/inner conventions.
It checks fresh decorated symbols against the committed generated bindings and
runs the convention tests both with and without the libffi build tag.

The [record example](../examples/cgorecords/main.go) runs with
`examples/run.sh <go|llgo> cgorecords <library>`. Fresh ordinary and packed
variants are checked on all eight Go targets and six llgo targets. Native tests
execute objects, archives and shared libraries at `-O0` and `-O2`: integer and
floating-point aggregates, padded/narrow/Boolean fields, nested arrays, large
returns, native address fields, record-valued native pointer prototypes and a
concrete variadic record prefix/result. Native struct-pointer factories,
mutation/identity/nulls and record-pointer function-pointer prototypes execute
without libffi. Additional tagged tests call these methods on independently
owned `NativeValue` storage. Nil/closed handles, incompatible targets
and packed C layout rejection are also checked without requiring libffi.

## Direct llgo bindings

`Header.LLGoSource(packageName, bindingType)` emits a binding type, a
`New<bindingType>(session)` constructor and typed methods. C cdecl scalars,
Boolean, native pointers, ordinary records and typed struct pointers are supported
on Linux/macOS/Windows amd64 and arm64. Record fields support nested records and
fixed arrays. Fixed and variadic cdecl function-pointer parameters/results retain their typed
prototypes. Other conventions and unqualified
386 targets fail generation explicitly.

Use `Declaration.WithTail(types...)` to select a concrete variadic method shape,
as with `CgoSource`. An unexpanded prefix emits a zero-tail call. Generated methods
keep their logical parameter types, promote narrow integers and Boolean values to
`int32` and `float32` to `float64`, and call a true C ellipsis pointer ending in
`__llgo_va_list ...any`. Fixed prefix types are unchanged; ordinary record prefixes
and results retain their compiler layout checks. This requires the merged
[llgo indirect-varargs fix](https://github.com/xgo-dev/llgo/pull/2766), included in CI's llgo `main`.

Direct narrow integer parameters require the merged
[llgo C ABI fix](https://github.com/xgo-dev/llgo/pull/2776). Native variadic pointer
locals also require the merged [debug snapshot fix](https://github.com/xgo-dev/llgo/pull/2783);
CI pins the main commit containing that fix. Runtime fixtures use
wide C `int` results so return truncation cannot hide
incorrect argument extension. They check signed minimum/maximum/negative/zero
values, unsigned high bits, both Boolean values, native factories and forwarding,
C-to-Go callback arguments, stack parameters with a hidden aggregate result, and
fixed variadic prefixes. Objects, archives and shared libraries execute at both
Clang `-O0` and `-O2` builds on the six qualified targets.

The constructor checks `Target.CheckHost` and compares generated record
`unsafe.Sizeof`, `unsafe.Alignof` and `unsafe.Offsetof` values with the saved Clang
layout before resolving any exports. This validates llgo storage without loading
libffi; dynamic `ForHost` still validates its own libffi layouts. Packed layouts
that differ from llgo storage fail construction. The constructor then resolves
every requested export into a `Symbol`. Each method casts the address to its generated
`//llgo:type C` function type inside `Symbol.WithAddress`. Calls therefore retain
code/library handles and reject a closed session; nil/zero bindings also return
`dylib.ErrClosed`. The caller continues to own the session and native pointer
storage. Typed pointers refer to actual native-compatible storage; there are no
`abi.AddressOf` temporary copies on this path. Native code must not retain borrowed
Go storage or managed pointers. The adapter adds no library registrations,
callback captures or returned pointer ownership. Build the generated source with
llgo and cgo, without libffi.
The source includes a `llgo && cgo` build constraint; use target file suffixes for
several generated variants in one package.

Methods and record fields capitalize an initial ASCII lowercase letter, or prefix
a leading underscore with `X`; ambiguous names fail. Record names are generated
as `<bindingType>Record<index>` in metadata order. Binding type names must not shadow Go
predeclared names or generated imports. The generator itself remains Go code.

Function-pointer types are named `<bindingType>Callback<index>`; identical saved
prototypes reuse a type, including variadic signatures with different concrete tails.
These are `//llgo:type C` native callable values. Variadic types retain only their
fixed prefix followed by `__llgo_va_list ...any`; callers must supply concrete
C-promoted tail values and match the native count/format contract. This supports
calling and forwarding native variadic entries, not exporting Go variadic callbacks.
A factory
method retains its symbol for the factory call only. Keep the producer image leased,
for example through `Symbol.WithAddress`, during all later invocations or while
passing the entry back to C. Returned entries and external callback addresses do
not acquire a new code owner. Noncapturing C literals can reenter the already
registered Go calling thread; this does not qualify them as foreign-thread entries.
Use separately owned `abi.NewCallback` registrations and address leases for
captures or C-created threads, with the optional libffi backend. Higher-order
entries retain nested native callback types in arguments/results. A multiply
indirect entry uses pointers to that callback type; these are borrowed slots,
not Go closure storage. Keep slot storage and referenced entries alive, and
synchronize slot mutation. Callback record fields use opaque native addresses
and the separate prototypes described above.
Noncapturing Go C entries with narrow integer results are not qualified by these
tests: their result-extension contract requires a separate compiler adapter. Use
`abi.NewCallback` or a compiled C bridge for that result boundary. Narrow results
from actual C entries are covered separately.
The [direct example](../examples/directdeclarations/main.go) and its tests execute
all integer widths, float/double, Boolean, native/null pointers, zero arguments,
void results and mixed calls, plus padded records, nested arrays, large and
floating-point aggregate returns, typed pointer mutation/identity, scalar/record/
large/pointer native factories, C callback invocation, local Go C entries and rejection
of a separately generated packed binding. CI compares fresh output with six
committed normal and packed target variants and executes the library example
without libffi. The same object/archive/library fixtures execute empty and mixed
variadic tails, all C default promotions, integer/floating-point register spills,
record prefixes/results, pointer mutation and native variadic factories/forwarding.
Raw aggregate copies can import C runtime helpers such as `memcpy`;
the example supplies process symbols on POSIX and explicitly loads the Windows
CRT. The core loader does not infer these dependencies.
`examples/run.sh llgo directdeclarations <library>` runs it;
`examples/declgen -llgo -var=Bindings` selects direct generation in the utility.

## Coverage and limits

| Host | Architecture | Go native calls | llgo native calls |
| --- | --- | --- | --- |
| Linux | amd64 / arm64 | Yes | Yes |
| Linux | 386 | Yes | Unqualified |
| macOS | amd64 / arm64 | Yes | Yes |
| Windows | amd64 / arm64 | Yes | Yes |
| Windows | 386 | Yes, including stdcall/fastcall | Unqualified |

Native CI compares committed generated Go files with fresh Clang output, compiles
those declarations with Go/llgo, and executes integer, floating-point, Boolean,
native-pointer, struct/struct-pointer and variadic calls. Runtime record parsing
executes mixed/padded records, nested arrays, floating-point aggregate returns,
large returns, variadic record prefixes, mutation and pointer identity.
Function-pointer tests execute captured scalar handlers, struct/large-record callbacks, typed
pointer mutation, native factories, variadic native pointers and Windows 386
stdcall/fastcall callbacks. It also executes C `long`/plain `char` signatures using compiler-confirmed widths/signedness. Foreign-target
tests inspect all eight targets without executing foreign code. Windows ARM64
also runs an independent Go process in the llgo job.

The top-level header is limited to 16 MiB, AST output to 32 MiB, diagnostics to
32 KiB, requested functions to 256, parameters per declaration to 256, records to
256, and total description nodes to 65,536. The existing descriptor limits of
eight nesting levels, 32 fields per struct and 65,536 expanded aggregate elements
apply. Compiler record storage is additionally limited to 64 KiB.
AST overflow terminates the compiler instead of accumulating output. Context
cancellation terminates active compiler processes. Invalid JSON, missing probes,
unsupported types, incompatible redeclarations and missing requested names fail
explicitly. No temporary compilation files are created by `Parse`.

This complements [llcppg](https://github.com/goplus/llcppg): its Clang-based static
bindings can use direct llgo C entries, while these generated descriptors resolve
symbols dynamically through this loader. Full C++ adapters, additional
function-pointer forms remain separate
work. Clang's
syntax and type information
come from its [AST interface](https://clang.llvm.org/docs/IntroductionToTheClangAST.html).
