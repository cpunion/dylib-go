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
| Typedefs and ordinary qualifiers | Desugared/resolved, including integer and pointer aliases |
| Object/opaque pointers | Native addresses; no automatic pointee allocation, copying or ownership |
| Ordinary structs, including anonymous typedef records | Arguments/results; compiler-evaluated layout checked against the native backend |
| Fixed array members | Scalars, pointers, records and nested arrays; positive evaluated lengths |
| Function parameters/results pointing to a complete ordinary struct | Typed single-struct pointees; `abi.AddressOf` supplies a temporary copy, with mutation/return identity checks |
| Function pointers at parameter/result boundaries | Typedefs, anonymous pointers and decayed function parameters; retain their exact prototype/convention independently of the outer pointer |
| Variadic prototypes | Fixed prefix; `Declaration.WithTail` adds a concrete scalar/pointer tail; `abi.Prepare` applies promotions |
| C cdecl | All eight qualified native targets |
| stdcall / fastcall | Windows 386; retain compiler symbol decoration |
| Unions, bitfields, attributed records/fields/typedefs, flexible/zero-length arrays, enums, function-pointer fields/multiple indirection/higher-order signatures, atomics, long double, extended integers or other conventions | Rejected by this generator; manual adapters remain independent |
| Static/inline functions and old-style declarations without prototypes | Rejected; provide an exported C facade/prototype |

`Header.Records` saves the C type spelling, `abi.TypeDesc` and compiler layout for
each used record. `LookupRecord(name)` clones that metadata for constructing values
with `abi.StructValue`/`abi.ArrayValue`. Clang declaration IDs associate anonymous
records with their typedef names. A second compiler invocation evaluates
`sizeof`, `_Alignof` and `__builtin_offsetof` with the same header/target/flags.
Generated source retains all layout checks. Compiler packing flags may produce
inspectable metadata, but `ForHost` rejects a backend layout mismatch.

Pointers within record members remain opaque addresses, including recursive
record links. Incomplete record pointers also remain opaque. Complete struct
pointers at function boundaries describe one pointee; array bounds, retained
storage, allocation and ownership are not inferred from a pointer prototype.
Use native addresses or an explicit manual descriptor for other pointee storage.
C array parameters decay to pointers; bare arrays are not passed by value.

`Declaration.FunctionPointers` stores each function-pointer signature with its
zero-based parameter `Position`; `-1` identifies a function-pointer result.
`LookupFunctionPointer(position)` returns a detached snapshot. The outer signature
uses `abi.Pointer`, so pass a leased `abi.NewCallback` address to C. `ForHost`
checks records used by both the outer call and each function-pointer signature.
The generated example derives its struct callback signature from these metadata.

Clang `__typeof__` probes expose `FunctionProtoType` nodes in the same AST
invocation. Their parameter/result types and effective calling convention avoid
parsing nested C declarator text. Function-pointer signatures support ordinary
records and typed struct pointers, along with the existing primitive types.
Pointers within records, multiple function-pointer indirection and callbacks that
accept/return other function pointers require manual adapters.

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
bridges in an `import "C"` preamble. It supports cdecl scalar and opaque native
pointer arguments/results on all eight native Go targets and six qualified llgo
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

Constructors check `Target.CheckHost` before resolving exports. Methods run their
C bridges inside `Symbol.WithAddress`, retaining the image for the call and
rejecting closed sessions. Nil/zero bindings return `dylib.ErrClosed`. Bridge
parameters/results use 32-bit integer transport for smaller integer types and
Boolean values; C restores declared widths before the native call. This avoids
depending on narrow-parameter extension across the Go/C boundary. Native pointer
typedefs carry explicit cdecl/stdcall/fastcall attributes on Windows 386,
independently for each outer/inner prototype. The static cgo bridge keeps the
ordinary host C convention. Records, typed struct pointees and other conventions
fail generation. Non-cdecl variadic calls and stdcall/fastcall on other targets
are rejected. Use dynamic
`Session.Bind` or the direct llgo record adapter for those supported interfaces.

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

The qualified compiler does not sign-extend negative `int8`/`int16` arguments
correctly for some optimized macOS ARM64 C calls, including native function
pointers ([llgo #2767](https://github.com/xgo-dev/llgo/issues/2767)). Narrow-result
round trips alone do not detect this: a C `int` result exposes the incorrect
positive value. Use `CgoSource`'s 32-bit transport or dynamic `Session.Bind` for
affected signatures until an upstream fix is qualified. The supported direct
fixture subset does not establish correctness for every scalar signature.

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
function pointers, multiple indirection and function-pointer record fields remain
unsupported by declaration parsing.
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
