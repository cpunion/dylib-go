# How DDL, ABIBridge, and llcppg complement each other

## Inspected versions

- [DDL `3bf531e`](https://github.com/Marenz/ddl/tree/3bf531e9701469ccecd5c3c698036ef4ef72362b): model, registry, linker, and ELF/OMF/COFF/archive components.
- [ABIBridge `4dfbda2`](https://github.com/lynnswap/ABIBridge/tree/4dfbda22afb9a1492e1c5c99933a9defbc5437a4): README, RuntimeArchitecture, CFunctionInvocation, CXXObjectInvocation, Architectures, and source responsibilities.
- [llcppg `6098773`](https://github.com/goplus/llcppg/tree/6098773c3116609e61c26968b83eec83e32a976c): configuration, Clang/mangling entry points, function/class generation, and sample output.
- Local llgo reference `8ac217053d0337b7261cebc923bb6b17955bf49a`; the original local compiler was a devel build. See the validation record; CI separately pins the [qualified compiler revision](ci.md), including public C-export thread guards.

## Responsibilities

| Task | DDL | dylib-go | ABIBridge | llcppg |
| --- | --- | --- | --- | --- |
| Load `.o/.a` and fix addresses | Core responsibility | Core Go implementation | Primarily loaded/system-loaded Apple images, rather than a general object linker | Outside scope |
| Resolve exported symbols from declarations | D-specific mangling, templates, reflection | Exact linkage names; no general demangler | Swift/C++ source-level names and candidate selection | Generate actual mangled bindings from headers and Clang |
| Call the C ABI | D templates and function pointers | Caller-defined cgo/llgo adapters; optional scalar/ordinary-struct libffi | libffi C backend, variadic calls, and Apple authenticated pointers | Generate llgo declarations, types, and calling conventions |
| C++ objects and methods | Does not solve general C++ ABI | Explicit C facade recommended; complex methods unimplemented | Supported receivers, methods, vtables, adapters, ownership | Clang-based classes, methods, layouts, and bindings; coverage depends on generator support |
| Swift/ObjC/SwiftUI | Outside scope | OS libraries with C facades; no native language ABI | Dedicated metadata, runtime, call, and value management | Mainly C/C++; no native Swift ABI adaptation |
| Library and handle lifetime | Libraries/modules | Sessions, generic symbol guards, bound calls, callback leases, optional retained references | Image leases, physical call plans, values, callback ownership | Primarily build/link-time semantics |

ABIBridge covers more than general libffi: Swift type metadata, generic witnesses, indirect results, async/throws, value lifetime, and authenticated pointers require language/platform knowledge. Its [runtime architecture](https://github.com/lynnswap/ABIBridge/blob/4dfbda22afb9a1492e1c5c99933a9defbc5437a4/Docs/RuntimeArchitecture.md) separates images, call plans, values, and callback ownership. This project uses a similar responsibility boundary without copying its Swift/C++ implementations.

Its [architecture documentation](https://github.com/lynnswap/ABIBridge/blob/4dfbda22afb9a1492e1c5c99933a9defbc5437a4/Sources/ABIBridge/ABIBridge.docc/Architectures.md) distinguishes metadata recognition, compilation, and device execution. Likewise, foreign-object parsing or relocation tests here do not establish execution on that CPU. ABIBridge's Apple focus does not automatically add Windows/Linux loaders.

llcppg commonly emits `//go:linkname F C.<mangled-name>` with `LLGoPackage` link flags, binding symbols when the host is built and linked. Its [function generator](https://github.com/goplus/llcppg/blob/6098773c3116609e61c26968b83eec83e32a976c/cl/func.go) distinguishes global/static functions from methods with `this`. Existing output does not establish runtime rebinding support, but its header semantics and type information are suitable inputs for dynamic adapter generation.

## Remaining gaps

The comparisons below use the pinned source snapshots, not all historical releases or project plans. DDL and ABIBridge have not been built or run as part of this project's validation. Their source and documented fixture coverage establish what to investigate; dylib-go's own [support matrix](support.md) establishes its execution evidence.

### Compared with DDL

dylib-go implements the general object-loading path: identify content, model sections and symbols, extract archive dependencies, relocate, bind native entries, and release the image. Its eight native Go targets and six llgo targets include modern 64-bit ELF/Mach-O/COFF and i386 ELF/COFF subsets. This is not yet full feature parity with DDL's library and plugin abstractions.

| Area | Evidence in the DDL snapshot | dylib-go today / remaining work |
| --- | --- | --- |
| Legacy formats and containers | `DefaultRegistry` enables OMF object/library, ELF object, and InSituMap loaders. COFF, ar, D `.ddl`, and InSituLib registrations are commented out; ELF64 is explicitly rejected | No OMF loader or host MAP import; D `.ddl` is excluded. OMF is a legacy-format gap. Modern COFF/ar/Mach-O/ELF execution is covered by this project's tests; the historical all-format plan is not a completed DDL baseline |
| Extensible loaders and metadata | `LoaderRegistry.register` composes loaders; libraries expose attributes and optional version-checking code | Parsers are internal and selected by content. No public loader/provider registration, plugin manifest, or version-contract API |
| Dependency discovery | `PathLibrary` searches directories and translates D namespaces into paths; the linker can consult a registry for dependencies | Explicit inputs and archive dependency extraction only. Add all dependencies before linking; no directory provider or manifest-based dependency discovery |
| Lazy providers and compilation | The separate `xf/linker` layer has object/source providers, compiler configuration, and symbol-triggered library loading | Archive members are lazy, but source compilation and provider-driven acquisition are absent. These belong in an optional package rather than the core parser |
| Library/module lifecycle | `DynamicLibrary` exposes unloading; `xf/linker/LazyLinker.unload(source)` removes a source from linker indexes | A successful session is sealed, and `Close` releases the entire image. Independent module removal, rebinding, and replacement require a dependency/lifetime model; DDL's index-removal code alone is not evidence of safe hot reload |
| Typed language discovery | D mangling/demangling, template symbol binding, ClassInfo, class construction, and subclass queries | Exact symbols and explicit C signatures only. Declaration-driven C/C++ binding is a useful extension; D class reflection and runtime compatibility remain intentionally excluded |
| Initialization | `Linker.initModule` visits imported D ModuleInfo records and calls constructors; destructor registration is a TODO | Modern ELF/Mach-O/COFF tables and session-owned C/C++ exit registrations; no D ModuleInfo. Legacy forms, unwind, and TLS still need work |

Source anchors: [default registry](https://github.com/Marenz/ddl/blob/3bf531e9701469ccecd5c3c698036ef4ef72362b/ddl/DefaultRegistry.d), [ELF limits](https://github.com/Marenz/ddl/blob/3bf531e9701469ccecd5c3c698036ef4ef72362b/ddl/elf/ELFBinary.d), [loader registry](https://github.com/Marenz/ddl/blob/3bf531e9701469ccecd5c3c698036ef4ef72362b/ddl/LoaderRegistry.d), [path library](https://github.com/Marenz/ddl/blob/3bf531e9701469ccecd5c3c698036ef4ef72362b/ddl/PathLibrary.d), [lazy linker](https://github.com/Marenz/ddl/blob/3bf531e9701469ccecd5c3c698036ef4ef72362b/xf/linker/LazyLinker.d), [compiler/provider setup](https://github.com/Marenz/ddl/blob/3bf531e9701469ccecd5c3c698036ef4ef72362b/xf/linker/DefaultLinker.d), [typed library access](https://github.com/Marenz/ddl/blob/3bf531e9701469ccecd5c3c698036ef4ef72362b/ddl/DynamicLibrary.d), and [module initialization](https://github.com/Marenz/ddl/blob/3bf531e9701469ccecd5c3c698036ef4ef72362b/ddl/Linker.d).

Separately, broader raw-object compatibility still needs Mach-O coalescing, remaining relocations, TLS, nonstandard import tables, and native unwind registration. ELF COMDAT, COFF selections 1–7/weak aliases, bigobj/extended relocations, and short/GNU long import libraries are implemented. The remaining items are dylib-go gaps, not a claim that DDL completely implemented them. The host OS supplies initialization, TLS, dependencies, and unwind metadata for complete shared libraries; it does not adapt an incompatible call signature or language value.

### Compared with ABIBridge

ABIBridge provides native language calls and runtime interpretation above Apple image loading. It does not replace this project's general raw `.o/.a` linker. Its [package requirements](https://github.com/lynnswap/ABIBridge/blob/4dfbda22afb9a1492e1c5c99933a9defbc5437a4/README.md) cover Apple platforms and a Swift/Xcode toolchain; that scope does not establish Linux/Windows ABI support.

| Area | ABIBridge in the inspected snapshot | dylib-go today / remaining work |
| --- | --- | --- |
| Dynamic C calls | Explicit signatures, variadic tails with C promotions, and reusable prepared call interfaces; no fixed argument-count limit | Host C ABI plus Windows 386 stdcall/fastcall; scalars, ordinary C structs, concrete variadic shapes, and owned reusable CIFs. Argument storage follows the concrete signature length; additional aggregate forms and conventions remain |
| Declaration resolution | Source-level Swift/C++ declarations, overload selection, image scopes, and shared metadata indexes | Exact linkage names; Go-style parsing supplies types but does not discover native prototypes. Header/declaration manifests or generated adapters can add validation |
| C++ receivers and virtual entries | Direct methods, owned/borrowed receiver storage, explicit base views, and adapter-described virtual tables/authentication | No built-in method/object ABI. Callers need C facades or compiled adapters; nontrivial C++ values also require compiler adapters in ABIBridge |
| Swift native ABI | Supported native values, metadata/witnesses, generic binding, direct/indirect results, async/throws, and explicit value operations | Only tested C exports from complete libraries. Native Swift signatures, managed values, generics, tasks, and errors need a dedicated runtime adapter |
| Objective-C and SwiftUI | Runtime dispatch, ownership, method hooks, and a separate SwiftUI adapter product | Raw registration objects are rejected; no message-dispatch, ARC, or UI adapter. A C facade alone does not supply object ownership or thread rules |
| Native callbacks and replacement | Native callback entry owners, supported Swift closures, import/method/virtual hooks, and publication/retirement contracts | Fixed C callback entries support scalars, ordinary structs, Go captures, and C-created threads, with leases and explicit unregister/join/close. No import/method/virtual hooks, Swift closures, or automatic registration/library ownership |
| Values, caches, and concurrency | Image leases, receiver/value owners, bounded physical call-plan caches, and per-call storage allow reuse of prepared handles | Session lifetime guards and native temporary copies are implemented. Session and prepared-plan calls support concurrency and callback reentry; close retires owners and waits for active calls. Exact-signature bindings share session-owned plans and lazy typed pointee layouts. Independent native value owners and broader physical cache normalization remain |
| Authenticated Apple ABIs | arm64e pointer authentication, compiler-specific virtual-call schemas, and documented device probes | arm64e execution is rejected. Supporting it needs authenticated callable pointers and matched toolchains/runtime backends; normal arm64 execution is not evidence of arm64e compatibility |

The [C invocation guide](https://github.com/lynnswap/ABIBridge/blob/4dfbda22afb9a1492e1c5c99933a9defbc5437a4/Sources/ABIBridge/ABIBridge.docc/CFunctionInvocation.md), [C++ object guide](https://github.com/lynnswap/ABIBridge/blob/4dfbda22afb9a1492e1c5c99933a9defbc5437a4/Sources/ABIBridge/ABIBridge.docc/CXXObjectInvocation.md), [Swift invocation guide](https://github.com/lynnswap/ABIBridge/blob/4dfbda22afb9a1492e1c5c99933a9defbc5437a4/Sources/ABIBridge/ABIBridge.docc/SwiftFunctionInvocation.md), and [runtime architecture](https://github.com/lynnswap/ABIBridge/blob/4dfbda22afb9a1492e1c5c99933a9defbc5437a4/Docs/RuntimeArchitecture.md) describe these contracts and their limits. ABIBridge also requires explicit native layouts, ownership, and adapters for unsupported representations; it is not automatic adaptation of every C++ or Swift declaration.

ABIBridge reports routine runtime tests on macOS and iOS Simulator with selected Xcode versions, separate public/native consumers, and compilation checks for other Apple device SDKs. Its [architecture record](https://github.com/lynnswap/ABIBridge/blob/4dfbda22afb9a1492e1c5c99933a9defbc5437a4/Sources/ABIBridge/ABIBridge.docc/Architectures.md) documents separate arm64e device probes, arm64e.x1 compilation without runtime verification, and protected import pages that could not be mutated. These records do not establish every device/ABI combination or universal hook support, and are not dylib-go test results.

### Priorities for this project

1. Broaden general raw-object use: Long import tables, Mach-O coalescing and remaining relocations, remaining lifecycle forms, unwind registration, then explicit TLS models. Modern lifecycle tables and session-owned C/C++ exit registrations are implemented. Keep parsing and linking in Go.
2. Broaden dynamic C interfaces: additional aggregate forms, calling conventions, and generated declaration validation. Fixed C callbacks with explicit retirement, foreign-thread integration, variadic promotion, and reusable call plans are implemented. Use llgo or small native bridges where Go cannot emit the required ABI entry points.
3. Add optional declaration/provider packages: generated C/C++ adapters using llcppg or Clang information, dependency manifests, and a lifecycle model before module replacement. Preserve the current explicit core API.
4. Evaluate an optional Apple C facade backed by ABIBridge for Swift/ObjC/C++ runtime values, using opaque handles and explicit retain/release. Add platform/runtime/device tests before claiming support.

OMF could be added separately if legacy object compatibility is needed; it is not part of the current roadmap. D `.ddl` and D runtime compatibility remain outside scope. See the [roadmap](roadmap.md) for target-specific work.

## Possible integration

```text
Headers / Clang / llcppg                 Apple runtime / ABIBridge adapter
  types, prototypes, mangled names         native Swift/ObjC adaptation
                  \                       /
                 explicit declarations and ownership
                              |
             dylib-go Session + Resolve + Bind
                 /                         \
         Go object linker                OS library loader
                 \                         /
       caller-defined adapter / optional scalar/struct libffi call
```

An optional [Clang declaration generator](clang.md) now supplies target-qualified
scalar/typedef/record/pointer signatures for dynamic binding, including compiler/
backend layout validation, array members, typed struct pointers and function-pointer
parameter/result prototypes. Generated struct callbacks and native factories are
executed with Go/llgo. Fixed scalar/record/native-pointer cdecl methods can also
be generated for direct llgo calls without libffi, with Clang/llgo storage checks.
C++ and direct callback/variadic adapters remain separate work.

The following integrations remain proposals:

1. Add an optional dynamic mode to llcppg. Preserve its generated types, names, and layouts, and generate binding fields owned by a session. Known signatures can use caller-defined `//llgo:type C` function types through `Symbol.WithAddress`. The loader need not duplicate header parsing.
2. Define a declaration manifest with target triple, C/C++ ABI, linkage name, arguments/results, record size/alignment, `this` and hidden result parameters, ownership, and exception boundaries. Existing `abi.Signature` covers concrete fixed/variadic scalar and ordinary C struct signatures with supported native C conventions; it does not express these additional language contracts.
3. Generate small `extern "C"` facades for complex C++ classes. A matching compiler handles construction, destruction, inheritance, results, and conversion of exceptions into error codes. Load the resulting objects or libraries through this project.
4. Use ABIBridge on a Swift/ObjC++ side of an Apple-specific adapter and expose a small C API. Go stores opaque handles with explicit retain/release and main-thread requirements. Such adapters and runtime dependencies are not currently included.
5. Use `abi.NewCallback` and its leases for fixed C callback entries. An integration must separately own the registering library, remove native registrations, and join callers before releasing the lease. Swift closures, import/method hooks, and language exception adaptation still need dedicated adapters. See [callback lifetime](callbacks.md).

C/C++ SDKs with headers, plugin engines, and numerical libraries are natural applications for generated adapters. SwiftUI, Swift generics/async, and Objective-C dispatch require dedicated language adapters and are outside current support.
