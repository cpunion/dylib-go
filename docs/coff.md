# COFF symbol and section selection

The Go linker implements these rules for AMD64, ARM64, and i386 COFF. Inputs still need supported relocations and a matching Windows host for execution.

## Ordinary objects and bigobj

Ordinary COFF/PE parsing uses Go's `debug/pe`. An independent Go reader handles bigobj's 56-byte header, format UUID, 32-bit section numbers, and 20-byte symbol records, following the [LLVM COFF format definitions](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/llvm/include/llvm/Object/COFF.h). Both paths then use the same symbol, COMDAT, relocation, and lifecycle implementation.

Ordinary COFF section references above 32767 remain positive; only reserved values are interpreted as signed. Bigobj associative section records combine the low/high parent fields. Direct objects and archive members are accepted by content. Section data, symbol/string tables, auxiliary counts, and relocation ranges are checked before access.

Extended relocation tables use the first record as a count marker, including that marker in the stored count. It is validated and excluded from actual relocations. This works for ordinary objects and bigobj. Bigobj does not widen relocation fields: a 16-bit SECTION relocation cannot encode an image ordinal above 65535. It also does not relax image size, W^X, architecture, TLS, or unwinding limits.

## Short import libraries

Short import objects use a 20-byte header followed by the public symbol, DLL name, and optional export-name string. Their signature resembles bigobj, but their version and content select a separate Go parser. Ordinary `.lib` containers can mix short imports with regular COFF members. Only selected imports open a DLL; unused missing dependencies remain inert.

`Inspect` reports `Info.Imports` with the DLL, normalized public name, export name or ordinal, and code/data/const kind. Inspection opens no dependency and is independent of the host architecture.

| Import name policy | DLL lookup |
| --- | --- |
| ORDINAL (0) | Header ordinal; no name lookup |
| NAME (1) | Original public name |
| NOPREFIX (2) | Remove one leading `?`, `@`, or `_` |
| UNDECORATE (3) | Remove that prefix and truncate at the first `@` |
| EXPORTAS (4) | Separate export-name string |

These transformations operate on the original linker name before normalizing the i386 C underscore. They follow [Microsoft's short import format](https://learn.microsoft.com/en-us/windows/win32/debug/pe-format#import-library-format) and [LLD's import implementation](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/lld/COFF/InputFiles.cpp). The loader does not infer calling conventions from decorations.

CODE imports expose the callable public name and `__imp_` IAT name. DATA exposes the IAT name. CONST exposes both names as aliases of the IAT slot. Slots are populated before protection becomes read-only, including import-only sessions. Calls use existing branch stubs when the DLL entry is out of range. Imports resolve against the declared DLL even if another loaded library exports the same name. Ordinary object definitions override callable import names; IAT names keep their DLL binding. Equivalent imports share definitions, while conflicting explicit DLL imports fail.

`Define` can supply the native function/data address for an import's normalized public name. Reserved exit/lifecycle helpers use the session-owned implementation, including imports through IAT slots. For other dependencies, explicitly loaded matching DLL names win; otherwise search `Options.LibraryPaths`, the importing file's directory, then native loader paths. DLL names are basenames; directories belong in `LibraryPaths`. An omitted extension defaults to `.dll`. The options slice is copied by `New`.

Each opened DLL reference belongs to the session. Link failure publishes no raw image and retains opened DLLs for retry, matching explicitly loaded dependencies. `Close` unloads them after raw finalization unless `KeepLibraries` requests retention. Native DLL initialization is an OS-loader side effect and cannot be rolled back by the raw linker.

Long import objects containing `.idata` tables remain unsupported for raw execution. They are recognized and reported in member metadata, so an unused descriptor member in a short import library does not block linking. This distinction prevents executing a thunk whose OS import table was never initialized. ARM64EC hybrid imports, delay-load tables, and linker directives for automatic default-library selection remain outside the current implementation.

## Weak externals

A COFF weak external has an auxiliary record naming its fallback symbol and archive search policy. It is not just an undefined ELF weak symbol. The supported policies follow the [Microsoft PE/COFF specification](https://learn.microsoft.com/en-us/windows/win32/debug/pe-format#auxiliary-format-3-weak-externals):

| Policy | Archive extraction for the weak name | Fallback |
| --- | --- | --- |
| NOLIBRARY (1) | No | Used when no selected definition or explicitly supplied native symbol exists |
| LIBRARY (2) | Search before following the fallback | Used if archive search cannot supply the weak name |
| ALIAS (3) | No | Used when no selected definition or explicitly supplied native symbol exists |

An ordinary reference to the same public name follows its weak alias. Fallbacks can refer to another alias, a selected object, or an archive definition. Selected strong definitions and explicit native symbols override fallbacks. SECTION/SECREL relocations and lifecycle dependency ordering follow the resolved provider too. Conflicting fallback declarations, malformed auxiliary records, and cycles reached by a used reference fail before initialization. ARM64EC anti-dependency policy is outside the supported target set.

## COMDAT

| Selection | Behavior |
| --- | --- |
| NODUPLICATES (1) | Reject duplicate keys |
| ANY (2) | Retain the first input |
| SAME_SIZE (3) | Require matching sizes, then retain the first input |
| EXACT_MATCH (4) | Require matching section size and raw contents, then retain the first input |
| ASSOCIATIVE (5) | Retain only with the parent COMDAT |
| LARGEST (6) | Retain the largest section; ties retain input order |
| NEWEST (7) | Retain the largest COFF header timestamp; ties retain input order |

EXACT_MATCH compares raw contents without comparing relocation targets or alignment, matching [LLD's link.exe-compatible implementation](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/lld/COFF/InputFiles.cpp). Discarded sections and their references never participate in dependency extraction or initialization. Inputs are snapshotted so an unsuccessful link can be retried.

NEWEST is an explicit loader policy for selection value 7; current LLD rejects this value. It uses the unsigned header timestamp, not file modification time or input order. Reproducible objects with equal or zero timestamps retain the first definition. This does not claim that current Microsoft/LLVM toolchains emit NEWEST objects.

## Validation

Clang-generated fixtures inspect all three COFF targets. Tests cover malformed records, alias chains/cycles, strong/native overrides, fallback archive extraction, address and section relocations, equal/unequal EXACT_MATCH content, and timestamp/tie selection. Existing Windows Go and llgo suites execute weak function calls from objects and archives and functions selected by EXACT_MATCH/NEWEST. Windows 386 executes with Go because setup-llgo does not install that architecture.

Large fixtures are genuine Clang output with 33000 and 65540 padding sections, testing ordinary unsigned indexes, bigobj, high associative parents, weak fallback indexes, and archive parsing. Windows native tests call functions from high-section bigobj objects/archives and verify the last of 65536 relocated data pointers. Separate malformed-input tests cover header/UUID/version, sizes and offsets, auxiliary counts, and overflow markers.

Import tests cover all name policies and CODE/DATA/CONST metadata on every host. Windows jobs build a real DLL and `.lib` with Clang/LLD, execute imported functions/data, and test ordinal lookup, export aliases, explicit DLL reuse, moved-library search paths, unused missing DLLs, failed links, and retry. Go and llgo use the same runtime tests; Windows 386 runs Go.
