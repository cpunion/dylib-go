# C declarations and generated dynamic bindings

`compiler/clang.Parse(ctx, header, options)` uses an optional external Clang to
preprocess a C17 header, evaluate primitive/record layout probes, and extract explicitly
requested function declarations. Go decodes the AST and renders source; no libclang
library or compiler dependency is added to the core loader.

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
declarations and executes a generated `sum_pair` struct binding through `Session.Bind`. Run
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
| Variadic prototypes | Fixed prefix; `Declaration.WithTail` adds a concrete scalar/pointer tail; `abi.Prepare` applies promotions |
| C cdecl | All eight qualified native targets |
| stdcall / fastcall | Windows 386; retain compiler symbol decoration |
| Unions, bitfields, attributed records/fields/typedefs, flexible/zero-length arrays, enums, function pointers, atomics, long double, extended integers or other conventions | Rejected by this generator; manual adapters remain independent |
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

Headers describe declarations rather than proving actual exports. The loaded
binary must match the header, compiler ABI and flags. DLL export aliases can
differ from object symbol decoration; choose the actual exported name explicitly.
No language runtime, C++ ownership or exception adaptation is inferred.

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
large returns, variadic record prefixes, mutation and pointer identity. It also
executes C `long`/plain
`char` signatures using compiler-confirmed widths/signedness. Foreign-target
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
symbols dynamically through this loader. Full C++ adapters, function-pointer/
callback declarations and direct llgo generation remain separate work. Clang's syntax and type information
come from its [AST interface](https://clang.llvm.org/docs/IntroductionToTheClangAST.html).
