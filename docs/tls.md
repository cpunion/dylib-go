# Raw-object TLS

Both modes use image-owned native OS-thread values and the same native cleanup
registry. Parsing, layout, validation, archive selection and relocation stay in
Go. Complete shared libraries continue to use their OS loader's TLS support.

| Raw input model | Option | Qualified scope |
| --- | --- | --- |
| Native Mach-O TLV | `Options.NativeTLS` | Plain C, macOS amd64/arm64; initialized, zero-filled and aligned values |
| Clang emulated TLS | `Options.EmulatedTLS` | Linux/macOS/Windows targets below, including documented C++ initialization/destruction |
| Native ELF dynamic TLS | `Options.NativeTLS` | Plain C, Linux amd64/386 GD/LD and arm64 TLSDESC; Clang and GCC |
| Native ELF static TLS, COFF TLS, other Darwin models | None | Unsupported; use emulated TLS or a complete OS library |

## Native Mach-O TLV

Compile ordinary C macOS objects with native TLS (Clang's default for the
supported targets, or explicit `-fno-emulated-tls`) and enable
`Options{NativeTLS: true}`. All participating TLS definitions must be loaded into
the same image, including lazily selected archive members. Failed links before
native initialization permit adding a missing provider and retrying.

The loader accepts `S_THREAD_LOCAL_REGULAR`, `S_THREAD_LOCAL_ZEROFILL` and
`S_THREAD_LOCAL_VARIABLES`. It validates each three-word TLV descriptor and its
bootstrap/template relocations before native initialization. Templates are
packed into one bounded, aligned module template after ordinary relocations,
including initialized structs, arrays and native pointers. Every native thread
receives an independent copy. The allocation budget is shared with emulated
TLS when both options are enabled; the existing image and alignment limits
also apply. At most 4096 native TLV descriptors are accepted per image.

Clang's `__thread_vars` can declare byte alignment. Private image pages and
24-byte descriptor strides still guarantee pointer-aligned runtime storage;
that normal compiler output is accepted. Zero-fill sections never read object
header bytes from their on-disk zero offset.

The linker binds `_tlv_bootstrap` to an image-owned entry generated in Go. The
entry calls the existing native per-thread allocator, then adds the validated
variable offset. It preserves Darwin's additional TLS registers: all GPRs
except RDI/RAX on amd64, and X1-X15 plus all 128 bits of Q0-Q31 on arm64, along
with ordinary callee-saved registers. No Go callback, private llgo runtime hook,
new C runtime interface or dyld TLS-module registration is needed. The original
compiler-emitted TLV callers remain intact.

Supported instruction relocations are zero-addend RIP-relative 64-bit MOV loads
(`X86_64_RELOC_TLV`) and zero-addend ADRP/LDR references
(`ARM64_RELOC_TLVP_LOAD_PAGE21/PAGEOFF12`). Targets must be complete selected
image-owned TLV descriptors. Ordinary references to template/descriptor bytes,
foreign or OS-library TLV descriptors, nonzero keys, bootstrap overrides,
unsupported opcodes/addends and partial/overlapping descriptors fail before
publication. Thread-local pointer and initializer sections, dynamic TLS
constructors/destructors, authenticated arm64e and native COFF models are
not qualified by this option. Linux ELF models have the separate scope below.

`Lookup`, `Resolve` and `Bind` reject raw TLS variable names: their linker address
is a descriptor, not a process-global variable address. Export a native C getter
or function that performs the TLS access instead. `DefineSymbol` can retain such
a function provider and its TLS owner across sessions; it does not transfer TLS
descriptors between images. Pin Go callers with `runtime.LockOSThread` for any
sequence that requires stable TLS identity, including initialization/finalization
when relevant. Stop native users and retain the corresponding code leases for
as long as borrowed TLS pointers remain in use.

Native tests cover O0/O2 objects/archives, retry, register and indirect-result
preservation, independent images, struct/pointer/array templates, mixed emulated
TLS, C-created threads, pinned Go threads, constructor/exit-hook/finalizer access,
retained function providers and owner retirement before/concurrent with thread
exit. Both architectures compare generated entry bytes with an independent LLVM
assembly reference. Native register probes check the full additional GPR/vector
contract, including both 64-bit lanes of every ARM64 vector register.

## Native ELF dynamic TLS

Enable `Options{NativeTLS: true}` for ordinary plain C Linux PIC objects and
archives. Clang and GCC global-dynamic/local-dynamic output is supported on
amd64/386; AArch64 uses the descriptor model emitted by both compilers. All
TLS definitions must be selected into the same image. Ordinary object/archive
dependency resolution, missing-provider retry and retained function providers
work as for Mach-O. Raw TLS symbol names cannot be bound as ordinary variables;
use a native getter, pin multi-call Go sequences to an OS thread and retain code
leases while borrowing returned TLS pointers.

| Architecture | Implemented static relocations | Address-entry convention |
| --- | --- | --- |
| amd64 | `TLSGD`, `TLSLD`, `DTPOFF32`, `DTPOFF64` | `__tls_get_addr`, ordinary System V C ABI |
| 386 | `TLS_GD`, `TLS_LDM`, `TLS_LDO_32` | GNU `___tls_get_addr`, index in EAX, result in EAX |
| arm64 | `TLSDESC_ADR_PAGE21`, `TLSDESC_LD64_LO12_NC`, `TLSDESC_ADD_LO12_NC`, `TLSDESC_CALL` | Descriptor in X0, result relative to `TPIDR_EL0`; preserves X1-X29 and every bit of Q0-Q31 |

The loader retains TLS symbol types, packs writable allocated `SHF_TLS` data and
zero-fill sections into one bounded aligned template, and applies ordinary
relocations before copying that template. Each native thread receives its own
module allocation through the existing image owner. Private GD/LD indices and
AArch64 descriptors refer to that owner; they never contain host dynamic-loader
module IDs. The linker generates all entry instructions in Go, including the
386 register-to-cdecl adapter and AArch64's additional register saves and
thread-pointer subtraction. No glibc DTV mutation, static TLS reservation,
private llgo runtime access or new C runtime interface is required.

Address relocations require the corresponding zero-addend instruction forms
(RIP-relative LEA uses the ABI's `-4` field addend). AArch64 register operands
remain those selected by the compiler, including GCC's alternate call register.
TLS definitions, offsets, template extents, instructions, alignment, reserved
helpers and allocation limits are validated before publication. Up to 4096
private address records are permitted; the 64 MiB per-thread allocation budget
is shared with emulated TLS. Compiler-produced local-dynamic module-base indices
are image-wide, and DTP-relative values use the packed template offsets.

Initial-exec/local-exec offsets, x86 TLSDESC, other AArch64 dynamic sequences,
loader-owned dynamic TLS relocations, external/OS-library TLS definitions,
native C++ initialization/destruction and SVE/SME calling variants
remain unsupported. They need separate ABI/runtime qualification. Complete OS
libraries continue to use their loader. Native ELF tests cover the same values,
thread isolation, cleanup, lifetime and lifecycle cases as Mach-O, plus GCC
O0/O2 calls and explicit rejection of static TLS models. Independent assembly
references cover all three entries; an AArch64 probe checks X1-X18 and all 128
bits of Q0-Q31.

ABI references: [AArch64 ELF TLS relocations](https://github.com/ARM-software/abi-aa/blob/main/aaelf64/aaelf64.rst),
[LLVM AArch64 ELF TLS register contract](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/llvm/lib/Target/AArch64/AArch64CallingConvention.td),
[GNU i386 address-entry convention](https://github.com/bminor/glibc/blob/master/sysdeps/i386/dl-tls.h)
and [AArch64 descriptor resolver](https://github.com/bminor/glibc/blob/master/sysdeps/aarch64/dl-tlsdesc.S).

## Emulated TLS

Compile every participating raw C translation unit with Clang `-femulated-tls`
and create the session with `Options{EmulatedTLS: true}`. This flag replaces
native TLS accesses with ordinary descriptor references and calls to
`__emutls_get_address`; it does not require new TLS relocation instructions.
The parser, archive selection, descriptor validation and relocation remain in
Go. A small native helper supplies allocation and thread-exit cleanup for both
Go and llgo hosts without private runtime imports. Cleanup itself needs no Go
callback; native destructor bodies can use retained `abi.NewCallback` entries.

| Input / runtime feature | Support |
| --- | --- |
| ELF amd64/arm64/386, Mach-O amd64/arm64, COFF amd64/arm64/386 | Explicit Clang emulated TLS in raw objects and selected archive members |
| Initialized / zero-filled variables | Independent template copy / zeroed storage for each native thread |
| Local / shared external variables | Retained image descriptors; ordinary symbol resolution and lazy archive dependencies |
| Alignment | Positive power of two; allocation budget includes alignment padding |
| Clang C++ TLS | Constant values, nonthrowing constructors and the thread-exit registrations below |
| Other native TLS models / OS TLS registration | Outside this emulated mode; plain C Mach-O TLV and the documented ELF dynamic models use `NativeTLS` above, and complete shared libraries use the OS loader |
| External emutls or thread-registration runtime overrides | Rejected; callbacks and descriptors must use the helper retained by this image |
| Windows fiber switching | Not qualified; storage and cleanup use Windows FLS |

| Object ABI / platform | Architectures | Thread-exit interface |
| --- | --- | --- |
| ELF / Linux | amd64, arm64, 386 | `__cxa_thread_atexit`, `__cxa_thread_atexit_impl` |
| Mach-O / macOS | amd64, arm64 | Clang `_tlv_atexit`; explicit C++ ABI registration also uses the same image owner |
| COFF / Windows GNU ABI | amd64, arm64, 386 | Clang `__cxa_thread_atexit` |
| COFF / Windows MSVC ABI | amd64, arm64, 386 | Clang emulated TLS `.CRT$XDU`, `__dyn_tls_on_demand_init` and zero-argument `__tlregdtor` callbacks |

MSVC ABI support requires Clang `-femulated-tls`; it does not load native MSVC TLS
sections, PE TLS directories or `.CRT$XL*` loader callbacks. `.CRT$XDU` entries
are validated and snapshotted separately from global constructors. On the first
access by a native thread, the runtime marks its image-owned byte guard before
calling that image's selected thread initializers. Reentry observes that guard;
other images and threads have separate guards. The private descriptor stays in
writable image data near the code. There are at most 4096 initializer entries.
Active constructors retain the image during close retry, even before they have
registered a destructor. Plain `void(void)` and C++ ABI `void(void*)` destructors
keep their actual function types and share the same native LIFO registry.
On Windows i386, Clang GNU registers automatic destructors with `thiscall`,
while manual C registrations use `cdecl`. A shared native adapter supplies
the single context in both ECX and the stack, aligns the call, and restores its
frame. Global C++ ABI exit registration uses the same adapter. No convention
is guessed from symbol names.

macOS C++ destructor execution is qualified with LLVM 22. Older producers can
mix native TLS accesses into `-O0 -femulated-tls` output: Apple Clang 17 and
upstream LLVM 18 can leave an undefined `__tls_guard` and native TLV relocations
alongside emulated descriptors. These mixed C++ outputs remain unqualified by `NativeTLS`; compile
them with a qualified producer. The native tests use `DYLIB_LLVM_CLANG` on macOS
when provided, leaving the other C fixtures on their original compiler.

Descriptors contain four native words: size, alignment, a mutable runtime word,
and a template pointer. Before publishing code or running constructors, the
linker requires correctly aligned writable non-executable descriptor storage,
an unused runtime word, bounded size/alignment and a template contained in
retained image data. Descriptors cannot overlap. There are at most 4096 distinct
descriptors and at most 64 MiB of allocation including padding per native thread,
shared with the native TLS module when both modes are enabled.
Validated size/alignment/template pointers are copied into private native controls.
Templates may contain ordinary relocated native pointers; their pointees still
need the appropriate lifetime and synchronization.

`__emutls_get_address` always binds the host-owned helper. Descriptors cannot be
supplied with `Define`/`DefineSymbol` or imported from a DLL, and the helper
cannot be replaced by an archive or host definition. The flag is explicit so a
previously unresolved TLS helper cannot accidentally select a process-global
runtime with incompatible ownership.

Values belong to native threads. A Go goroutine can move between native threads;
use `runtime.LockOSThread` around a sequence that requires stable TLS identity.
Initializers and finalizers execute on their invoking threads; callers requiring
one thread across initialization, use and close must pin that whole sequence.
This API does not turn native TLS into goroutine-local storage.

The helper uses one host-lifetime pthread key or FLS slot. Thread exit drains
native callbacks in LIFO order before releasing values. A callback is detached
before invocation and runs outside the registry lock, so callbacks can read TLS
and register more callbacks. The key continues to identify the same storage
during this dispatch. Each native thread permits up to 4096 queued callbacks
across images. Native registration returns `-1` for null functions, allocation
failure or that limit. Registration hooks always require `EmulatedTLS`, including
manual calls with no compiler descriptors; they never select a process-global
thread runtime. The address helper still requires validated image descriptors.

Session close retires calls and waits for code leases, then drains this image's
callbacks on the invoking native thread before global finalizers. Cleanup remains
on one native thread across its native calls. Global exit hooks and thread
callbacks can register each other; cleanup drains new registrations before
releasing either registry. If another thread still has queued or executing
callbacks or active MSVC thread initializers, `Close` returns
`ErrPendingThreadDestructors`, retains the image,
unwind tables, TLS values and providers, and rejects new session calls. Join
those threads and retry `Close`; completed global finalizers do not repeat.
Keep callback/value/code leases needed by destructor bodies until their cleanup
has finished. Native exceptions must not escape these callbacks.

After callbacks finish, close releases all values for the image, including
trivial values on still-live threads. Thread state retains no retired descriptor,
template or code pointer. Its small empty record survives until thread exit and
is reused by later images. Native thread exit and owner retirement share a lock;
pending and executing callbacks prevent their owner from being freed. Native
allocation failure during TLS access aborts, as in the compiler runtime; owner
creation failures return a link error before publication.

A TLS pointer borrowed by native code requires a code lease for as long as it or
the object can be used. Stop/unregister/join native users before releasing that
lease. `Close` can reclaim values after native code has stopped even if an OS
thread remains alive; it cannot make a stale borrowed TLS pointer valid.

`TestEmulatedTLSAcrossTargets` compiles all eight targets and Windows GNU variants.
MSVC fixtures inspect all three Windows targets at O0/O2 and execute in the native
Windows Go and llgo jobs. Native tests cover O0/O2 object/archive
calls, shared external descriptors, constant/zero values, 64-byte alignment,
dependency retry, repeated load/close, C-created threads, thread-exit reclamation,
constructor/exit-hook/finalizer access, retained code leases and owner retirement
before or concurrently with native thread exit. C++ cases include nonthrowing
construction, reverse destruction order, recursive registration, reentrant TLS
reads, per-image isolation, cleanup retry during queued/executing callbacks,
manual registrations and the callback budget. The optional ABI suite calls
captured Go handlers during ordinary foreign-thread work and again during
thread exit, exercising runtime reentry after native thread cleanup has begun.
Pure Go tests validate 32/64-bit descriptors and metadata without executing code.
MSVC cases also cover multi-object/archive initializer tables, per-thread shared
guards, constructor callbacks reentering Go and raw code, and retirement while
constructors are blocked. A host C fixture independently exercises the common
zero-argument initializer helper, recursive guard use and mixed destructor types.

ABI references: [Clang emulated TLS option](https://clang.llvm.org/docs/UsersManual.html#cmdoption-femulated-tls)
and [LLVM 22 descriptor/runtime ABI](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/compiler-rt/lib/builtins/emutls.c).
The image-owned runtime here is separate from compiler-rt's process-global owner.

Native TLV ABI references: LLVM 22 [Darwin AArch64 register mask](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/llvm/lib/Target/AArch64/AArch64CallingConvention.td) and [Darwin x86-64 register mask](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/llvm/lib/Target/X86/X86CallingConv.td).
