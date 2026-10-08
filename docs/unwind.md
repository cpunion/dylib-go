# Raw C frame registration

`Options{RegisterUnwind: true}` enables raw C frame registration on Windows
amd64/arm64 and Linux amd64/arm64/386. Compile C fixtures with `-funwind-tables`
so the compiler emits `.pdata/.xdata` or `.eh_frame`. Complete OS libraries
continue to use their OS-managed unwind registration.

| Target | Accepted raw metadata |
| --- | --- |
| Windows amd64 | 12-byte `.pdata` records and version 1 `.xdata`, without language handlers or chained records |
| Windows arm64 | 8-byte `.pdata`; packed function/fragment records, or version 0 full/extended `.xdata`, without language handlers |
| Linux amd64/arm64/386 | ELF `.eh_frame` and `.eh_frame.*`, DWARF32 CIE versions 1/3, ordinary C augmentation, and the CFI subset below; libgcc_s required |
| Windows 386, macOS | Raw registration remains pending; requesting it for a raw image returns an error |

## Windows

Go validates relocated ranges and metadata, merges retained `.pdata` and
`.pdata$*` arrays, rejects overlaps, and sorts the private output table by
function start. Function ranges must lie inside executable image sections.
Metadata must lie in aligned data sections. AMD64 code slots/operand counts,
ARM64 packed lengths, full/extended headers, and epilog scope bounds are checked.
Unwind instructions still describe compiler-generated native code; these
checks do not verify each instruction against its machine-code prolog.

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
CIE augmentations are empty or `zR`. Address fields can use native pointers or
4/8-byte fixed signed/unsigned values, absolute or PC-relative. Indirect,
text/data-relative and variable-length address encodings are rejected.

The initial CFI subset includes advance-location, offset/restore, undefined,
same-value, register, remember/restore state, CFA register/offset, signed
variants, value-offset and GNU argument-size instructions. Expressions,
`set_loc`, language personalities/LSDA and target-specific instructions remain
unsupported. Structural validation does not verify CFI against machine code.

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
This backend covers C stack traversal with libgcc; it does not establish C++
exception support or compatibility with a separately selected LLVM unwinder.

## Defaults and scope

The option defaults to false. Default loading discards COFF and ELF unwind
sections and their relocations/dependencies as before. With registration enabled,
unsupported versions, handler/chained records, CIE augmentations and invalid
metadata fail before initialization. Leaf functions can legitimately have no table entries.

This extension supports native C frame lookup and stack traversal. C++/SEH
language handlers, RTTI/runtime adapters, Windows 386 SEH, other POSIX unwind
providers, and Mach-O DWARF/compact unwind require further work. Native exceptions must remain
within compatible native runtime boundaries; callers must retire raw native
activity before closing a session.

CI's existing Windows amd64/arm64 Go and llgo jobs execute real objects and
archives. A DLL observer captures a context and uses `RtlVirtualUnwind` through
two raw C frames, returning a checked result. Tests verify OS table lookup,
unregistration after close, and registration ownership during failed integer
initialization cleanup. Every pure Go job inspects real cross-target compiler
records and rejects malformed/unsupported table cases.

References: [Microsoft x64 unwind format](https://learn.microsoft.com/en-us/cpp/build/exception-handling-x64),
[Microsoft ARM64 unwind format](https://learn.microsoft.com/en-us/cpp/build/arm64-exception-handling),
[runtime table registration](https://learn.microsoft.com/en-us/windows/win32/api/winnt/nf-winnt-rtladdfunctiontable),
and [runtime table deletion](https://learn.microsoft.com/en-us/windows/win32/api/winnt/nf-winnt-rtldeletefunctiontable).

Linux references: [GCC frame registration](https://github.com/gcc-mirror/gcc/blob/releases/gcc-15/libgcc/unwind-dw2-fde.c), [GCC unwinder](https://github.com/gcc-mirror/gcc/blob/releases/gcc-15/libgcc/unwind-dw2.c), and [LLVM registration contracts](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/llvm/lib/ExecutionEngine/Orc/TargetProcess/RegisterEHFrames.cpp).
