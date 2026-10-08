# COFF symbol and section selection

The Go linker implements these rules for AMD64, ARM64, and i386 COFF. Inputs still need supported relocations and a matching Windows host for execution.

## Ordinary objects and bigobj

Ordinary COFF/PE parsing uses Go's `debug/pe`. An independent Go reader handles bigobj's 56-byte header, format UUID, 32-bit section numbers, and 20-byte symbol records, following the [LLVM COFF format definitions](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/llvm/include/llvm/Object/COFF.h). Both paths then use the same symbol, COMDAT, relocation, and lifecycle implementation.

Ordinary COFF section references above 32767 remain positive; only reserved values are interpreted as signed. Bigobj associative section records combine the low/high parent fields. Direct objects and archive members are accepted by content. Section data, symbol/string tables, auxiliary counts, and relocation ranges are checked before access.

Extended relocation tables use the first record as a count marker, including that marker in the stored count. It is validated and excluded from actual relocations. This works for ordinary objects and bigobj. Bigobj does not widen relocation fields: a 16-bit SECTION relocation cannot encode an image ordinal above 65535. It also does not relax image size, W^X, architecture, TLS, or unwinding limits. Short import objects use a related signature but remain a separate, unsupported input until DLL dependency handling is implemented.

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
