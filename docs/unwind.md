# Raw Windows runtime function tables

`Options{RegisterUnwind: true}` enables raw COFF runtime function table
registration on Windows amd64 and arm64. Compile C fixtures with
`-funwind-tables` so the compiler emits `.pdata/.xdata`. Complete OS libraries
continue to use their OS-managed unwind registration.

| Target | Accepted raw metadata |
| --- | --- |
| Windows amd64 | 12-byte `.pdata` records and version 1 `.xdata`, without language handlers or chained records |
| Windows arm64 | 8-byte `.pdata`; packed function/fragment records, or version 0 full/extended `.xdata`, without language handlers |
| Windows 386, Linux, macOS | Raw registration remains pending; requesting it for a raw image returns an error |

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

The option defaults to false. Default loading discards COFF unwind sections
and their relocations/dependencies as before. With registration enabled,
unsupported versions, handler/chained records and invalid metadata fail before
initialization. Leaf functions can legitimately have no table entries.

This extension supports native C frame lookup and stack traversal. C++/SEH
language handlers, RTTI/runtime adapters, Windows 386 SEH, POSIX `.eh_frame`,
and Mach-O compact unwind require further work. Native exceptions must remain
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
