# Raw frame registration and native exceptions

`Options{RegisterUnwind: true}` enables raw C frame registration on Windows
amd64/arm64, macOS amd64/arm64 and Linux amd64/arm64/386. Compile C fixtures
with `-funwind-tables`
so the compiler emits `.pdata/.xdata` or `.eh_frame`. Complete OS libraries
continue to use their OS-managed unwind registration.

| Target | Accepted raw metadata |
| --- | --- |
| Windows amd64 | 12-byte `.pdata` records and version 1 `.xdata`, including the chained C records below, without language handlers |
| Windows arm64 | 8-byte `.pdata`; packed function/fragment records, or version 0 full/extended `.xdata`, without language handlers |
| Linux amd64/arm64/386 | ELF `.eh_frame` and `.eh_frame.*`, DWARF32 CIE versions 1/3, C and the C++ personality/LSDA subset below; libgcc_s required |
| macOS amd64/arm64 | Mach-O `__eh_frame`, PC-relative DWARF32 C records, individually registered with system libunwind |
| Windows 386 | Raw registration remains pending; requesting it for a raw image returns an error |

## Windows

Go validates relocated ranges and metadata, merges retained `.pdata` and
`.pdata$*` arrays, normalizes supported AMD64 nested chains, and sorts the private output table by
function start. Function ranges must lie inside executable image sections.
Metadata must lie in aligned data sections. AMD64 code slots/operand counts,
ARM64 packed lengths, full/extended headers, and epilog scope bounds are checked.
Unwind instructions still describe compiler-generated native code; these
checks do not verify each instruction against its machine-code prolog.

AMD64 version 1 records may use `UNW_FLAG_CHAININFO`. Secondary records can
be empty or contain `UWOP_SAVE_NONVOL`/`UWOP_SAVE_NONVOL_FAR` integer-register
saves; stack allocation belongs to the primary record. Go follows each embedded
`RUNTIME_FUNCTION` iteratively, validating its executable range, aligned metadata,
prolog length, operands and matching frame register/offset. Cycles, truncated
tails, language handlers and other secondary operations fail before registration.
Shared primaries and multi-level chains are supported. Embedded parent records
need not have a separate `.pdata` entry.

LLVM's nested `.seh_startchained` ranges are accepted when each overlapping child
contains the enclosing entry in its unwind chain and begins after that parent's
prolog is complete. Go selects the innermost range and produces a sorted,
disjoint registration table. Equal starts, crossing ranges and unrelated overlaps
are rejected; a child may end at its parent's end.

When a parent resumes after a child, an empty chain points to its original
`RUNTIME_FUNCTION`. Reusing the original unwind address with a moved function
start would incorrectly treat the resumed code as another partial prolog. These
16-byte proxy records use preallocated, non-executable image pages inside the
32-bit RVA window. The pages become read-only before registration and remain
owned through deregistration. Original section bytes are not rewritten.
This does not add language exception handling or version 2/3 unwind formats.

The output table lives in OS-owned read-only memory. Go calls
`RtlAddFunctionTable` after link/root validation and before initialization.
Registration stays alive through all active calls, exit registrations and
static termination tables, including cleanup after a failed C initializer.
`Close` calls `RtlDeleteFunctionTable` before freeing the table or image. If
deletion fails, native storage is retained and an error is returned.

## Linux

Go validates relocated `.eh_frame` records, preceding CIE references, encoded
function ranges inside executable sections, augmentation boundaries, LEB128
operands, and CFI operand storage before invoking a native unwinder. Accepted
CIE augmentations are empty or start with `z`, followed by distinct `P`, `L`
and/or `R` fields. `L` requires `P`. Go checks each augmentation's declared
length, consumes its fields exactly, and rejects unknown or repeated fields.
Address fields can use native pointers or 4/8-byte fixed signed/unsigned values,
absolute or PC-relative. Personality and LSDA pointers may additionally use
indirection through an image-owned data cell or allocated GOT slot. Encoded
zero remains null before applying a base or dereferencing a slot, following
libgcc. FDE code ranges cannot use indirection. Text/data-relative and
variable-length address encodings are rejected.

The initial CFI subset includes advance-location, offset/restore, undefined,
same-value, register, remember/restore state, CFA register/offset, signed
variants, value-offset and GNU argument-size instructions. Expressions,
`set_loc` and target-specific instructions remain
unsupported. Structural validation does not verify CFI against machine code.

With `RegisterUnwind` enabled, the linker retains read-only
`.gcc_except_table`/`.gcc_except_table.*` sections and their relocation
dependencies. Non-null FDE LSDA pointers must reference these backed,
non-executable sections. A personality must be non-null and executable if
inside the raw image; external personalities come from explicitly supplied
runtime dependencies. The native personality interprets the trusted LSDA
contents and type information. This loader does not validate the C++ type
system, action tables, or compatibility between separately supplied runtimes.

ELF COMDAT selection also removes FDEs whose initial-PC relocation names
discarded code. It remaps retained records, CIE back-references, relocation
offsets and zero-size frame symbols in a private snapshot. Non-frame references
into a rewritten frame section and sized frame symbols are rejected. Failed
links and retries leave staged input bytes unchanged.

Each selected object's read-only frame section gets four additional zero bytes
in its private mapping. This terminates libgcc's section scan even when the
original section occupies a whole page. Registration uses `__register_frame`
and `__deregister_frame` from one retained `libgcc_s.so.1` handle. It does not
mix functions from different unwind runtimes or assume LLVM's per-FDE contract.
The provider handle and frame sections remain alive through initialization,
calls, and successful/failed cleanup. Sections are deregistered in reverse
order before their image and provider are released. libgcc's void registration
interface has no recoverable status channel.

Linux Go and llgo amd64/arm64 jobs, and native Go 386, execute `_Unwind_Backtrace`
through two raw C frames in objects and archives. A separately retained library
queries `_Unwind_Find_FDE` before and after close. Tests also cover default
loading, validation retry and failed integer-initializer ownership. Pure Go
jobs inspect real ELF32/ELF64 compiler output and reject malformed records.
The same jobs execute C++ `throw`, typed `catch`, rethrow and RAII cleanup
across two raw objects, reversed object order and lazy archives at `-O0` and
`-O2`. They also catch a throw from an explicitly loaded shared library,
check destructor counts, retry a missing root, and execute through a retained
code lease during session retirement. FDE lookup succeeds until the lease is
released and fails after close. Default loading still discards exception data.
Pure Go tests inspect C++ output for all three ELF targets and reject malformed
augmentation, pointer, LSDA storage and COMDAT frame cases.

These tests use Clang C++17 with `-fPIC -fexceptions -stdlib=libstdc++
-rtlib=libgcc -unwindlib=libgcc` and an explicitly loaded runtime-linked native
library. A call must catch its exceptions in compatible native code before
returning to Go or llgo. Unwinding through Go frames or Go callbacks, C++ TLS,
arbitrary language runtimes, and a separately selected LLVM unwinder are not
qualified. Supplying an exception-enabled object does not discover or adapt
its runtime automatically.

## macOS

The Mach-O path retains `__eh_frame` records and reuses the Go DWARF validator.
It requires PC-relative FDE addressing. Explicit SUBTRACTOR/UNSIGNED pairs use
the normal linker. Some x86-64 assembler records have no relocations: before
W^X protection, Go maps their original text addresses to the image's section
layout and rewrites the relative field in private native storage. Staged input
bytes remain intact for retries.

Each FDE is registered separately with `__register_frame` from a retained
`/usr/lib/system/libunwind.dylib` handle. macOS does not receive libgcc's
section-start registration or zero-terminator contract. Deregistration uses
the same provider, in reverse order after cleanup and before unmapping.
The metadata-only `S_COALESCED` type of `__eh_frame` is accepted without
extending general Mach-O symbol/section coalescing.

Only emitted DWARF records are registered. Compact unwind conversion and
compact-only functions remain pending; ordinary macOS C output can omit
DWARF when compact encoding suffices. The test fixture adds a harmless
`.cfi_escape 0x00` no-op to force real DWARF output on older compilers while
preserving the C stack layout. Consumers must arrange DWARF for the raw frames
they need to traverse. C++ personality/LSDA, signal frames, expression CFI,
and arm64e pointer-authentication variants remain outside this subset.

Existing macOS amd64/arm64 Go and llgo jobs execute object/archive traversal,
FDE lookup before/after close, default loading, validation retry and failed
initialization cleanup. Every pure Go job verifies real compiler records and
both explicit ARM64 and implicit x86-64 reference rebasing. macOS can fall
back to frame-pointer walking when no FDE exists; the default-path test checks
that no dynamic FDE is registered rather than requiring traversal to fail.

## Defaults and scope

The option defaults to false. Default loading discards COFF, ELF and Mach-O DWARF unwind
sections and their relocations/dependencies as before. With registration enabled,
unsupported versions, unsupported language handlers, chained records, CIE
augmentations and invalid metadata fail before initialization. Leaf functions
can legitimately have no table entries.

This extension supports native C frame lookup/stack traversal and the Linux
C++ subset above. Windows/macOS language handlers, general RTTI/runtime
adapters, Windows 386 SEH, other POSIX unwind
providers, and Mach-O compact unwind require further work. Native exceptions must remain
within compatible native runtime boundaries; callers must retire raw native
activity before closing a session.

CI's existing Windows amd64/arm64 Go and llgo jobs execute real objects and
archives. A DLL observer captures a context and uses `RtlVirtualUnwind` through
two raw C frames, returning a checked result. Tests verify OS table lookup,
unregistration after close, and registration ownership during failed integer
initialization cleanup. Every pure Go job inspects real cross-target compiler
records and rejects malformed/unsupported table cases.

Windows AMD64 Go and llgo jobs also execute a separately assembled two-level
chain through the same OS unwinder, with objects, archives, default loading,
per-range lookup, unregistration and validation retry. Pure Go jobs relocate
the MSVC and MinGW assembler metadata and validate shared/empty/FAR chains,
malformed records, and a 2048-level chain and cycle.

The same native jobs execute LLVM's overlapping two-level output, with objects,
archives, cleanup and retry. Seven controlled `RtlVirtualUnwind` contexts check
partial prologs, nested saves and both resumed parents, including restored
registers, stack pointer and return address. `VirtualQuery` verifies read-only
proxy storage. Pure Go tests check nesting, sibling and shared-end intervals,
frame registers, deterministic output, unchanged input sections and storage limits.

References: [Microsoft x64 unwind format](https://learn.microsoft.com/en-us/cpp/build/exception-handling-x64),
[Microsoft ARM64 unwind format](https://learn.microsoft.com/en-us/cpp/build/arm64-exception-handling),
[runtime table registration](https://learn.microsoft.com/en-us/windows/win32/api/winnt/nf-winnt-rtladdfunctiontable),
and [runtime table deletion](https://learn.microsoft.com/en-us/windows/win32/api/winnt/nf-winnt-rtldeletefunctiontable).

Linux references: [GCC frame registration](https://github.com/gcc-mirror/gcc/blob/releases/gcc-15/libgcc/unwind-dw2-fde.c), [GCC unwinder](https://github.com/gcc-mirror/gcc/blob/releases/gcc-15/libgcc/unwind-dw2.c), and [LLVM registration contracts](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/llvm/lib/ExecutionEngine/Orc/TargetProcess/RegisterEHFrames.cpp).

C++ metadata references: [LSB `.eh_frame` augmentations](https://refspecs.linuxfoundation.org/LSB_5.0.0/LSB-Core-generic/LSB-Core-generic/ehframechpt.html), [GCC encoded pointers](https://github.com/gcc-mirror/gcc/blob/releases/gcc-14.2.0/libgcc/unwind-pe.h), and [Itanium exception ABI](https://itanium-cxx-abi.github.io/cxx-abi/abi-eh.html).

macOS references: [Apple unwind interface](https://github.com/apple-oss-distributions/libunwind/blob/main/libunwind/include/unwind.h), [LLVM FDE registration](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/libunwind/src/libunwind.cpp), and [LLVM Mach-O definitions](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/llvm/include/llvm/BinaryFormat/MachO.h).
