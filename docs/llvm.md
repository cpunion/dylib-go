# LLVM IR and bitcode compilation

`compiler/llvm.Compile(ctx, input, options)` optionally invokes an external
`llc` to generate PIC object code. It snapshots a regular input file, validates
the output with the Go object parser and returns an owned `Object` containing
`Path` and `Info`. It does not add LLVM libraries to the core loader or start
a JIT. The compiler executable defaults to `llc`; set `Options.Compiler` to a
different installed version or path. `Options.TempDir` selects temporary storage.

The module supplies its target triple and data layout. The helper passes no
target override. A target-less module uses the LLVM tool's default target,
which may differ from the Go process, especially for 386 hosts using 64-bit
tools. Prefer explicit module targets. LLVM versions must be compatible with
the input producer. These behaviors and object emission are provided by the
[LLVM static compiler](https://llvm.org/docs/CommandGuide/llc.html).

## Coverage

Every native Go and llgo CI job compiles both text IR and bitcode produced by
its Clang toolchain, loads the resulting object, removes compiler files before
linking, and executes C exports and an initializer. Foreign-target tests inspect
all eight object targets on each installed LLVM backend; they do not execute
foreign code.

`DYLIB_LLC` selects the test compiler; `DYLIB_LLVM_CLANG` selects its IR/bitcode
producer (otherwise `CLANG` or `clang`). macOS Go jobs select a matched LLVM 22
pair without changing the C compiler used by the other native fixtures.

| Object target | Output | Go native call tests | llgo native call tests |
| --- | --- | --- | --- |
| Linux amd64 | ELF | Yes | Yes |
| Linux arm64 | ELF | Yes | Yes |
| Linux 386 | ELF | Yes | Unqualified |
| macOS amd64 | Mach-O | Yes | Yes |
| macOS arm64 | Mach-O | Yes | Yes |
| Windows amd64 | COFF | Yes | Yes |
| Windows arm64 | COFF | Yes, in the ARM64 llgo job | Yes |
| Windows 386 | COFF | Yes | Unqualified |

LLVM tools can generate other architectures and operating systems, but this
helper does not extend the loader's execution backends or relocation support.
ELF objects with `OSABI_NONE` do not uniquely identify their OS in `Info.OS`.
The compiler and execution tests remain separate from metadata inspection.

## Ownership and limits

Call `Session.Load(object.Path)` before `object.Close()`. The session snapshots
native object bytes, so compilation storage can be deleted before `Link` and
native execution. `Object.Close` removes only its owned directory and is
idempotent. Do not read the path concurrently with closing it. Session closing
and native code lifetime follow the normal loader rules.

The input snapshot is limited to 256 MiB and compiler diagnostics to 32 KiB.
Cancellation terminates the compiler process. Failed compilation or output
validation removes temporary files; cleanup errors are reported too. No shell
interprets file names or compiler paths. Successful object output is inspected
under the existing parser limits. Unsupported features can still be reported
in `Info.Unsupported` or rejected when linking.

Use `dylib.Options.LibraryPaths` for dependencies located beside the original module;
the compiler's temporary directory is the staged object's default search origin.
The [tested library example](../examples/llvm/main.go) makes that path explicit.
It runs through `examples/run.sh <go|llgo> llvm <module.ll|module.bc>` and uses a
declared C signature rather than inferring one from IR symbol names.

## Language and container scope

LLVM output from C, C++ facades, Rust C exports, llgo C exports or another
frontend can use this path when its version, native target, ABI and required
runtime services match. Compilation does not supply missing runtimes, foreign
TLS, exception machinery, garbage collector registration or language ABI
adaptation. Existing object initialization, imports, unwind and relocation
restrictions still apply. Keep exports at a supported C boundary.

Compile modules or archives explicitly. `Session.Load` still does not directly
load IR, bitcode or archives containing bitcode members. These helpers do not
merge modules, infer signatures, generate language facades or translate an
already lowered ABI between operating systems.

## Bitcode archive compilation

`CompileArchive(ctx, path, options)` converts ordinary GNU/SysV, COFF and BSD
archives containing raw/wrapped LLVM bitcode, native objects, or both. Each
bitcode member goes through `Compile` independently. Native members retain their
bytes, even when they contain embedded bitcode sections; this is not an LTO
pipeline. Native-only archives do not require an installed compiler.

The output is an owned `Archive` with `Path`, `Info` and idempotent `Close`.
Load its path into a session, close compilation storage, then link requested
roots. Members retain order and names, including duplicates and long names.
Compilation does not interpret names as filesystem extraction paths.

The loader rebuilds native symbol indexes and selects members on demand. Unused
members' unresolved native symbols and initializers remain unselected, while
all member contents must be valid for compilation/inspection. Heterogeneous
native targets can be inspected; execution still checks each selected member.
The input's bitcode index is discarded. The generated BSD-style ar has no ranlib
index; it targets this loader, and external static linkers may require indexing.

Input and generated archives each have a 256 MiB limit, including headers and
names; at most 4096 object members are decoded. Context cancellation terminates
an active compiler and stops further members. Any failure removes owned files.
Thin/proxy bitcode archives, nested archives and text IR archive members are
rejected. Thin compilation and LLVM module merging remain future work.

Set `dylib.Options.LibraryPaths` for original dependency directories; compiling
an archive does not copy DLLs or keep the original archive directory as the
session's search origin. The [tested archive example](../examples/llvmarchive/call.go)
makes this explicit. Native Go and llgo tests use genuine GNU/BSD archives with
mixed native/bitcode members, duplicate names, dependencies in reverse order,
selected/unused initialization, and source/artifact deletion before linking.
They run on the same execution targets in the coverage table above. Metadata
tests also preserve all eight targets and exercise raw/wrapped bitcode.

References: [LLVM archive containers and bitcode indexes](https://llvm.org/docs/CommandGuide/llvm-ar.html),
[LLVM bitcode wrappers](https://llvm.org/docs/BitCodeFormat.html#bitcode-wrapper-format).
