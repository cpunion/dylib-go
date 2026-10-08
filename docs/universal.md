# Universal Mach-O containers

`Inspect` accepts Apple's big-endian `FAT_MAGIC` and `FAT_MAGIC_64` wrappers.
`Info.Kind` is `universal`; `Info.Members` contains each slice's target and
metadata, with archive members nested inside their slice. Inspection does not
select a host architecture or execute code.

| Payload | macOS loading behavior |
| --- | --- |
| Relocatable Mach-O object | Select the host slice and use the existing Go linker |
| Regular ar archive of Mach-O objects | Select the host slice, then extract required objects using the existing archive linker |
| Mach-O dylib or bundle | Validate the host slice and open the original universal file with the OS loader |
| Executable | Inspect metadata; refuse library loading |
| Nested universal files, thin archive slices, non-Mach-O payloads | Rejected |

Native selection supports macOS amd64 and arm64. AMD64 selects the generic
x86_64 subtype; ARM64 prefers the generic subtype and can use ARM64_V8.
Specialized x86_64h and authenticated arm64e slices remain inspectable but are
not selected for raw execution. Other CPU instructions are still the caller's
responsibility: architecture metadata does not validate every instruction.
Mobile-platform slices and files without a matching baseline macOS slice fail
before staging an object or opening a library. Other OS hosts inspect these
containers but cannot execute their slices.

For shared libraries, there must be exactly one slice for the host CPU. The OS
chooses its own subtype; accepting multiple host variants could cause it to
load a different slice from the one validated here. Extract a supported
baseline slice when a dylib contains several variants for the same CPU.

Container validation checks table bounds, alignment, nonempty ranges, overlap,
duplicate CPU/subtype pairs, FAT64 reserved bits, payload architecture, and
consistent image types. Each archive slice must contain relocatable Mach-O
objects matching its declared CPU/subtype; non-macOS archive members prevent
native selection. Archive/image mixtures are rejected. The existing 256 MiB
file and 64 MiB linked-image limits apply, with at most 64 slices per container.

Selection adds no relocation, language runtime, TLS or raw unwind support.
Only the selected slice supplies object code and symbols. OS libraries retain
their normal dependency, initialization, runtime and ownership behavior.

CI cross-inspects both header widths on every pure Go host and tests foreign
execution refusal, offsets, subtypes and malformed input. The macOS Go and
llgo jobs execute real `lipo`-generated FAT32/FAT64 objects and archives, plus
universal dylibs. Each architecture's function returns a different result so
the tests verify that the selected slice actually executes.

References: [Apple fat format](https://github.com/apple-oss-distributions/cctools/blob/main/include/mach-o/fat.h),
[Apple subtype selection](https://github.com/apple-oss-distributions/cctools/blob/main/libmacho/arch.c),
and [Go's Mach-O fat reader](https://go.dev/src/debug/macho/fat.go).
