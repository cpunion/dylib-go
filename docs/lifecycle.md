# Raw object initialization and cleanup

`Load` stages raw objects. The first successful `Link`, `Lookup`, `Resolve`, or `Bind` relocates the selected objects, validates all lifecycle entries and requested roots, applies W^X protection, then runs their initializers. Repeated linking does not run them again. A failed validation runs no native initializer and permits a dependency fix and retry.

`Close` prevents further calls, releases prepared call plans, executes session-owned exit callbacks, runs termination arrays, drains any callbacks registered by those terminators, and only then frees the image and native registration storage. OS library handles remain available throughout this sequence and are released afterward. `KeepLibraries` retains OS handles, but does not retain the raw object image or skip its finalization. Closing an unlinked session runs no object code; repeated close is harmless.

## Supported tables

| Format | Initialization | Termination | Order |
| --- | --- | --- | --- |
| ELF32/ELF64 | `SHT_PREINIT_ARRAY`, `SHT_INIT_ARRAY` | `SHT_FINI_ARRAY` | Preinitializers first; numeric suffix priorities ascending, default 65535; finalizers reverse |
| Mach-O64 | `S_MOD_INIT_FUNC_POINTERS` | `S_MOD_TERM_FUNC_POINTERS` | Initializers forward; terminators reverse |
| COFF AMD64/ARM64/i386 | `.CRT$XC*` void initializer tables | `.CRT$XP*`, `.CRT$XT*` void terminator tables | Lexicographic subsection order; XP before XT, both forward |

Equal-priority entries use provider-before-consumer object order derived from retained relocation dependencies. Strongly connected components preserve input order. Entry order within a table is retained. Explicit ELF priorities and COFF names take precedence over dependency order; dependency cycles do not imply a language-specific solution to cyclic global initialization.

Only selected archive members participate. COMDAT-discarded arrays and their references do not participate. ELF numeric priorities must fit 16 bits. Tables must contain whole native pointers, cannot request executable storage, and may contain null sentinels. Every non-null entry must point into an executable section of this image; ARM64 entries must also be instruction aligned. External initializer pointers are currently rejected. The validated entry lists are copied before initialization, so later writes to a writable table cannot substitute a finalizer.

Legacy ELF `.ctors`/`.dtors` and executable `.init`/`.fini` sections, Mach-O `S_INIT_FUNC_OFFSETS`, COFF `.CRT$XI*` integer-returning initializers, and CRT TLS tables remain unsupported. This does not implement a complete CRT startup environment or register unwind information.

## C and C++ exit registrations

Undefined raw-object references to `atexit`, `__cxa_atexit`, `__cxa_finalize`, and `__dso_handle` use image-owned implementations. These names, including their `__imp_` forms, are reserved against `Session.Define`. An object's own definitions retain normal linker precedence; bringing a custom runtime implementation makes its lifetime the caller's responsibility. OS libraries retain their own runtime and registration rules.

The implementation preserves arguments and DSO tags for `__cxa_atexit`. `__cxa_finalize(tag)` consumes matching registrations; a null tag consumes all registrations in this image, not other sessions. Plain `atexit` callbacks and tagged registrations share a LIFO list. An entry is detached before invocation, so recursive finalization cannot invoke it twice. New registrations during finalization are visited. This supports ordinary global C++ construction and destruction without registering object code in the process-global exit list. It does not adapt C++ methods, exceptions, RTTI, TLS destructors, or `__cxa_thread_atexit`.

Go owns parsing, dependency ordering, table validation, native thunk encoding, and session lifetime. A small C bridge owns the native list, synchronizes native registrations, and invokes native function pointers. Context-bound thunks are implemented for SysV/Windows AMD64, AArch64, and i386 cdecl. The i386 thunk realigns the helper's stack for GNU C even when its MSVC caller supplies only four-byte alignment. No Go closure, Go pointer, or ambient thread-local session state is used. libffi is not required for initialization or cleanup; these paths run with both ordinary Go/cgo and llgo.

This is image cleanup, not arbitrary rollback of native effects or hot module replacement. Initializers and destructors have no Go error channel. They must return normally and must not re-enter the session's locking methods. Native code may start threads or publish raw addresses; callers must stop such activity before closing. `Close` cannot infer whether an external runtime still holds a code pointer. Mixed termination tables and registered callbacks use the sequence above, rather than promising every platform CRT's process-exit interleaving.

## Verification

The existing setup-go and setup-llgo native jobs execute C/C++ fixtures that verify dependency order in either load order, archive extraction without unused constructors, global destructors, late registrations, per-DSO and recursive termination, session isolation, explicit table order, idempotence, and validation failure before initialization. The observer is a separately retained OS library, so finalizer effects are checked after the raw mapping has been freed. Pure Go tests inspect lifecycle sections for all eight target formats and reject malformed or unsupported forms.

Reference contracts: [Itanium C++ ABI termination](https://itanium-cxx-abi.github.io/cxx-abi/abi.html#dso-dtor), [Microsoft CRT initialization](https://learn.microsoft.com/en-us/cpp/c-runtime-library/crt-initialization), [LLVM Mach-O section definitions](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/llvm/include/llvm/BinaryFormat/MachO.h), and [GNU linker initialization-priority sorting](https://sourceware.org/binutils/docs/ld/Input-Section-Wildcards.html).
