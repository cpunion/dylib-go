# Raw-object emulated TLS

Compile every participating raw C translation unit with Clang `-femulated-tls`
and create the session with `Options{EmulatedTLS: true}`. This flag replaces
native TLS accesses with ordinary descriptor references and calls to
`__emutls_get_address`; it does not require new TLS relocation instructions.
The parser, archive selection, descriptor validation and relocation remain in
Go. A small native helper supplies allocation and thread-exit cleanup for both
Go and llgo hosts, without Go callbacks or private runtime imports.

| Input / runtime feature | Support |
| --- | --- |
| ELF amd64/arm64/386, Mach-O amd64/arm64, COFF amd64/arm64/386 | Explicit Clang emulated TLS in raw objects and selected archive members |
| Initialized / zero-filled variables | Independent template copy / zeroed storage for each native thread |
| Local / shared external variables | Retained image descriptors; ordinary symbol resolution and lazy archive dependencies |
| Alignment | Positive power of two; allocation budget includes alignment padding |
| C++ constant-initialized trivial TLS | Same descriptor ABI; language initialization/destruction runtimes are not supplied |
| Native TLS sections, relocation models and OS TLS registration | Unsupported for raw objects; complete shared libraries use the OS loader |
| C++ thread destructors / external emutls runtimes | Unsupported; recognized registration references and runtime overrides are rejected |
| Windows fiber switching | Not qualified; storage and cleanup use Windows FLS |

Descriptors contain four native words: size, alignment, a mutable runtime word,
and a template pointer. Before publishing code or running constructors, the
linker requires correctly aligned writable non-executable descriptor storage,
an unused runtime word, bounded size/alignment and a template contained in
retained image data. Descriptors cannot overlap. There are at most 4096 distinct
descriptors and at most 64 MiB of allocation including padding per native thread.
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

The helper uses one host-lifetime pthread key or FLS slot. Each thread holds
native bookkeeping; thread exit releases its values and removes them from their
image owners. Session close waits for code calls/leases, executes native exit
callbacks and finalizers while TLS remains available, then releases all values
for that image, including values belonging to still-live threads. Thread state
then retains no image descriptor, template or code pointer. Its small empty
bookkeeping record survives until thread exit and is reused by later images;
repeated load/close does not accumulate retired value records. Native thread
exit and owner retirement share a native lock. No pthread/FLS destructor invokes
unmapped object code. Native allocation failure during access aborts, as in the
compiler runtime; registration failures return a link error before publication.

A TLS pointer borrowed by native code requires a code lease for as long as it or
the object can be used. Stop/unregister/join native users before releasing that
lease. `Close` can reclaim values after native code has stopped even if an OS
thread remains alive; it cannot make a stale borrowed TLS pointer valid.

`TestEmulatedTLSAcrossTargets` compiles all eight targets and checks actual C++
thread-destructor registration refusal. Native tests cover O0/O2 object/archive
calls, shared external descriptors, constant/zero values, 64-byte alignment,
dependency retry, repeated load/close, C-created threads, thread-exit reclamation,
constructor/exit-hook/finalizer access, retained code leases and owner retirement
before or concurrently with native thread exit. Pure Go tests validate 32/64-bit
descriptor boundaries and budgets without executing foreign code.

ABI references: [Clang emulated TLS option](https://clang.llvm.org/docs/UsersManual.html#cmdoption-femulated-tls)
and [LLVM 22 descriptor/runtime ABI](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/compiler-rt/lib/builtins/emutls.c).
The image-owned runtime here is separate from compiler-rt's process-global owner.
