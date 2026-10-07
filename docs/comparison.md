# How DDL, ABIBridge, and llcppg complement each other

## Inspected versions

- [DDL `3bf531e`](https://github.com/Marenz/ddl/tree/3bf531e9701469ccecd5c3c698036ef4ef72362b): model, registry, linker, and ELF/OMF/COFF/archive components.
- [ABIBridge `4dfbda2`](https://github.com/lynnswap/ABIBridge/tree/4dfbda22afb9a1492e1c5c99933a9defbc5437a4): README, RuntimeArchitecture, CFunctionInvocation, CXXObjectInvocation, Architectures, and source responsibilities.
- [llcppg `6098773`](https://github.com/goplus/llcppg/tree/6098773c3116609e61c26968b83eec83e32a976c): configuration, Clang/mangling entry points, function/class generation, and sample output.
- Local llgo reference `8ac217053d0337b7261cebc923bb6b17955bf49a`; the original local compiler was a devel build. See the validation record; CI separately pins llgo v1.0.6.

## Responsibilities

| Task | DDL | dylib-go | ABIBridge | llcppg |
| --- | --- | --- | --- | --- |
| Load `.o/.a` and fix addresses | Core responsibility | Core Go implementation | Primarily loaded/system-loaded Apple images, rather than a general object linker | Outside scope |
| Resolve exported symbols from declarations | D-specific mangling, templates, reflection | Exact linkage names; no general demangler | Swift/C++ source-level names and candidate selection | Generate actual mangled bindings from headers and Clang |
| Call the C ABI | D templates and function pointers | Caller-defined cgo/llgo adapters; optional scalar libffi | libffi C backend and Apple authenticated pointers | Generate llgo declarations, types, and calling conventions |
| C++ objects and methods | Does not solve general C++ ABI | Explicit C facade recommended; complex methods unimplemented | Supported receivers, methods, vtables, adapters, ownership | Clang-based classes, methods, layouts, and bindings; coverage depends on generator support |
| Swift/ObjC/SwiftUI | Outside scope | OS libraries with C facades; no native language ABI | Dedicated metadata, runtime, call, and value management | Mainly C/C++; no native Swift ABI adaptation |
| Library and handle lifetime | Libraries/modules | Sessions, generic symbol guards, bound calls, optional retained references | Image leases, physical call plans, values, callback ownership | Primarily build/link-time semantics |

ABIBridge covers more than general libffi: Swift type metadata, generic witnesses, indirect results, async/throws, value lifetime, and authenticated pointers require language/platform knowledge. Its [runtime architecture](https://github.com/lynnswap/ABIBridge/blob/4dfbda22afb9a1492e1c5c99933a9defbc5437a4/Docs/RuntimeArchitecture.md) separates images, call plans, values, and callback ownership. This project uses a similar responsibility boundary without copying its Swift/C++ implementations.

Its [architecture documentation](https://github.com/lynnswap/ABIBridge/blob/4dfbda22afb9a1492e1c5c99933a9defbc5437a4/Sources/ABIBridge/ABIBridge.docc/Architectures.md) distinguishes metadata recognition, compilation, and device execution. Likewise, foreign-object parsing or relocation tests here do not establish execution on that CPU. ABIBridge's Apple focus does not automatically add Windows/Linux loaders.

llcppg commonly emits `//go:linkname F C.<mangled-name>` with `LLGoPackage` link flags, binding symbols when the host is built and linked. Its [function generator](https://github.com/goplus/llcppg/blob/6098773c3116609e61c26968b83eec83e32a976c/cl/func.go) distinguishes global/static functions from methods with `this`. Existing output does not establish runtime rebinding support, but its header semantics and type information are suitable inputs for dynamic adapter generation.

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
          caller-defined adapter / optional scalar libffi call
```

The following integrations remain proposals:

1. Add an optional dynamic mode to llcppg. Preserve its generated types, names, and layouts, and generate binding fields owned by a session. Known signatures can use caller-defined `//llgo:type C` function types through `Symbol.WithAddress`. The loader need not duplicate header parsing.
2. Define a declaration manifest with target triple, C/C++ ABI, linkage name, arguments/results, record size/alignment, `this` and hidden result parameters, ownership, and exception boundaries. Existing `abi.Signature` covers scalar C signatures only.
3. Generate small `extern "C"` facades for complex C++ classes. A matching compiler handles construction, destruction, inheritance, results, and conversion of exceptions into error codes. Load the resulting objects or libraries through this project.
4. Use ABIBridge on a Swift/ObjC++ side of an Apple-specific adapter and expose a small C API. Go stores opaque handles with explicit retain/release and main-thread requirements. Such adapters and runtime dependencies are not currently included.
5. Future native callbacks require explicit lifetimes for the native entry, Go closure, library, and registration. A temporary function address is insufficient, and language exceptions must not cross ABI boundaries.

C/C++ SDKs with headers, plugin engines, and numerical libraries are natural applications for generated adapters. SwiftUI, Swift generics/async, and Objective-C dispatch require dedicated language adapters and are outside current support.
