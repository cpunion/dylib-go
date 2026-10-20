# Raw-object emulated TLS

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
| Native TLS sections, relocation models and OS TLS registration | Unsupported for raw objects; complete shared libraries use the OS loader |
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

macOS C++ destructor execution is qualified with LLVM 22. Older producers can
mix native TLS accesses into `-O0 -femulated-tls` output: Apple Clang 17 and
upstream LLVM 18 can leave an undefined `__tls_guard` and native TLV relocations
alongside emulated descriptors. Such objects cannot use this runtime; compile
them with a qualified producer. The native tests use `DYLIB_LLVM_CLANG` on macOS
when provided, leaving the other C fixtures on their original compiler.

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
