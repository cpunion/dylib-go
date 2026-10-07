# Go implementation of DDL's loading mechanisms

## Source verification

The inspected [Marenz/ddl snapshot](https://github.com/Marenz/ddl/tree/3bf531e9701469ccecd5c3c698036ef4ef72362b), commit `3bf531e9701469ccecd5c3c698036ef4ef72362b`, contains `ddl/`, `xf/linker/`, and `meta/`, and points to the original dsource DDL project. This mirror makes the core source available, but does not establish that the complete SVN history has been recovered.

In that snapshot, [DefaultRegistry.d](https://github.com/Marenz/ddl/blob/3bf531e9701469ccecd5c3c698036ef4ef72362b/ddl/DefaultRegistry.d) registers OMF, ELF, and InSituMap, while archive and COFF registrations are commented out. [ELFBinary.d](https://github.com/Marenz/ddl/blob/3bf531e9701469ccecd5c3c698036ef4ef72362b/ddl/elf/ELFBinary.d) explicitly rejects ELFCLASS64. Early plans and the presence of format directories should not be treated as evidence of complete implementation.

## Responsibility mapping

| DDL responsibility | Go implementation | Change |
| --- | --- | --- |
| `DynamicLibraryLoader` / `LoaderRegistry` content identification | `parse`, format parsers, `Session.Load` | Identify file content rather than guess from extensions |
| `DynamicLibrary` / `DynamicModule` | Internal `file/object/section/symbol/relocation` model | Go structs and standard parsers; no D class or module metadata |
| `Linker.link` dependency resolution | `selectObjects`, `definitions`, `image.symbol` | Deterministic strong/weak/common rules, common merging, duplicate errors |
| `ar/ArchiveReader` / `ArchiveLibrary` | `archive.go` | GNU/SysV, BSD, and COFF long names; rebuild indexes and extract on demand |
| Per-module `resolveFixups` | `relocate*.go` | 64-bit ELF/Mach-O/COFF and i386 ELF/COFF subsets, bounds checks, GOT, branch stubs |
| `host` / `insitu` | `Define`, explicit shared libraries, optional host symbols | Native addresses without D MAP/ModuleInfo |
| `Memory` and object lifetime | `internal/native`, `Session.Close`, `Symbol.WithAddress` | W^X, rollback, explicit ownership, serialized access |
| Template binding and D reflection | `Resolve`, caller-defined adapters, `abi.Signature`, `Function` | Explicit native signatures; no D ABI |
| D ModuleInfo constructors/destructors | Outside scope | OS libraries can initialize runtimes; recognized raw-object requirements are rejected |

This is a functional reimplementation rather than a line-by-line syntax translation. OMF records, D mangling, class exports, and the Enki/meta tools were not copied. Modern relocation handling was independently implemented from the format specifications listed below.

## Loading sequence

1. `Load` parses objects and stages archives. Shared libraries are opened immediately by the OS, so `Load(shared)` may execute library constructors.
2. The first `Link(roots...)`, `Lookup`, `Resolve`, or `Bind` selects all explicit objects and required archive members. The definition index is rebuilt after each extracted member until dependency closure stabilizes.
3. Global strong definitions take precedence over weak/common. Multiple strong definitions fail. Common symbols use the maximum size and alignment. Local symbols retain object scope.
4. One contiguous mapping contains sections, common storage, GOT, and branch stubs. Each section occupies its own pages to separate writable and executable permissions. Images are limited to 64 MiB and files to 256 MiB. Section alignment larger than a host page is rejected.
5. Go applies relocations and validates ranges. Nearby stubs can reach distant native function addresses; arbitrary data references cannot be replaced by branch stubs. GOT slots hold addresses for indirect references.
6. Instruction caches are flushed and page permissions are applied. The image is published only after all steps succeed. Failure frees the mapping and allows dependency fixes followed by retry. Side effects of opening shared libraries are outside this image rollback.
7. `Close` frees the object image, then releases OS library handles in reverse order. `KeepLibraries` deliberately retains library references until process exit.

Lookup does not infer types. Raw addresses from `Lookup` are valid only while the session remains open. There is no finalizer that unloads code at an arbitrary GC-selected time.

## Generic binding and lifetime

`Resolve(name)` returns an untyped `Symbol` associated with its owning session. `Symbol.WithAddress` invokes a Go adapter while holding the session lock, so `Close` cannot release code or libraries during address use. The adapter may invoke a native function with a known signature or access native data with a known layout. It must finish all address use before returning and must not re-enter the session's locking methods.

`Bind(name, abi.Signature)` builds a `Function` on the same symbol lifetime guard. It validates and copies the caller's signature, then uses the optional libffi backend to arrange scalar and ordinary C struct arguments and results. Neither API infers types from names or demangled strings.

The independent `abi/signature` package converts Go-style declarations and typed invocations into explicit `abi.Signature` and `abi.Value` descriptions using Go's standard parser. It has no native backend dependency. The CLI consumes that metadata through `Bind`; syntax parsing does not replace native prototype or calling-convention knowledge. See the [CLI reference](cli.md).

Struct metadata is deep-copied when binding. The backend asks libffi for native field offsets and alignment and marshals individual values into C-owned memory. Temporary pointees preserve alias identity within one call and are copied back afterward. Returned pointers to a whole temporary pointee become logical snapshots; interior temporary pointers fail instead of exposing freed addresses. Numeric native pointers retain caller-managed ownership. Scalar-only calls retain their existing bridge without struct marshaling.

Signature-specific methods do not belong to the loader's core contract. The experimental `BindInt32` / `CallInt32` methods and `Int32Func` type were removed. Their small adapter now lives in `examples/call`, used by the demonstration CLI and compiler probes. Applications can write their own adapters, and generators such as llcppg can emit them from declarations.

## Go first, with llgo for native calls

Parsing, archives, symbol tables, layout, errors, relocations, signature descriptions, and lifetime ownership are implemented in Go. The default module has no external Go dependencies. `CGO_ENABLED=0` preserves inspection.

POSIX mappings and protections use Go `syscall`; Windows uses `VirtualAlloc`, `VirtualProtect`, and `VirtualFree`. POSIX `dlopen/dlsym/dlclose` and instruction-cache flushing use small C bridges.

Ordinary gc callers use a typed cgo bridge to enter the C ABI correctly. The [cgo example](../examples/cgo/main.go) supplies a mixed integer/floating-point signature. A Go `func` value's representation cannot be treated as a C pointer.

The [llgo example](../examples/llgo/main.go) defines a caller-owned `//llgo:type C` function type and invokes a native pointer directly. Known signatures can therefore use compiler-generated calls without the optional dynamic ABI backend. Unknown-at-build-time signatures can use `abi` with libffi; libffi arranges registers and stack arguments, but does not parse or link binaries.

## Format references

- [Go debug/elf](https://pkg.go.dev/debug/elf), [debug/macho](https://pkg.go.dev/debug/macho), and [debug/pe](https://pkg.go.dev/debug/pe): file structure parsing.
- [Arm ELF64 ABI](https://github.com/ARM-software/abi-aa/blob/main/aaelf64/aaelf64.rst): AArch64 ELF relocations and alignment.
- [Microsoft PE/COFF](https://learn.microsoft.com/en-us/windows/win32/debug/pe-format): COFF/PE, archives, and AMD64/ARM64/i386 relocations.
- [LLVM Mach-O definitions](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/llvm/include/llvm/BinaryFormat/MachO.h): format constants and structures.
- [LLVM JITLink](https://llvm.org/docs/JITLink.html): a comparison for long-term backend scope; its implementation is not used as a runtime backend here.
