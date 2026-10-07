# Platforms, compilation targets, and language outputs

The tables distinguish file inspection, implemented relocations, and actual execution evidence. Passing samples do not establish support for every relocation, TLS model, exception mechanism, or language ABI on a platform.

## Host platforms

| Host / target | Raw objects | System libraries | Evidence and limits |
| --- | --- | --- | --- |
| macOS arm64 | Mach-O 64 `.o` / `.a` | `.dylib` / bundle | Go and llgo execution tests, including optional libffi |
| macOS amd64 | Mach-O 64 `.o` / `.a` | `.dylib` | Go and llgo on GitHub Intel runners; additional local Rosetta tests |
| Linux arm64 | ELF64 LE RELA `.o` / `.a` | `.so` | Go and llgo on GitHub ARM64 runners |
| Linux amd64 | ELF64 LE RELA `.o` / `.a` | `.so` | Go and llgo on GitHub AMD64 runners |
| Windows amd64 | AMD64 COFF `.obj` / ordinary `.lib` archives | PE `.dll` | Native Go and llgo objects, archives, DLLs, and scalar ABI calls |
| Windows arm64 | COFF inspection; raw object execution rejected | PE `.dll` | Independent Go and llgo hosts, DLL/libffi calls, and Go/llgo c-shared producers; ARM64 COFF relocation unimplemented |
| Linux / Windows 386 | Inspection; 32-bit object execution rejected | No execution support in this project | CI runs 32-bit Go parser/refusal tests; setup-llgo does not install 386 |
| Linux RISC-V, LoongArch, PPC, s390x, ARM32 | Standard parsers can inspect some ELF inputs | No execution support in this project | No matching relocation backend or execution tests |
| FreeBSD and other BSD systems | Pure Go inspection can be built | No native backend | Needs OS operations, ABI validation, and execution tests |
| iOS / tvOS / watchOS / visionOS | Some Mach-O metadata can be inspected | No host integration | Mobile objects are identified and rejected as macOS plugins; execution policies also apply |
| Wasm, GPU, eBPF, bare-metal MCU | Outside native input support | Not applicable | Requires a separate executor, driver, or kernel loader |

Pure Go inspection does not require a file's CPU to match the host. Execution requires matching architecture, width, and format, and checks available OS metadata. Common ELF `OSABI_NONE` inputs do not identify their OS/libc, so their headers cannot prove ABI compatibility. Callers also remain responsible for required CPU features. Mach-O CPU subtypes are retained, and unimplemented authenticated ABIs such as arm64e are rejected.

## Files and relocations

| Input | Current behavior |
| --- | --- |
| Supported ELF/Mach-O/COFF 64-bit relocatable objects | Go links implemented relocations directly without first producing an OS shared library |
| Ordinary GNU/BSD ar (`.a`, ordinary COFF `.lib`) | Inspect all members, but map only needed members; unresolved dependencies in unused members do not affect linking |
| `.so`, `.dylib`, PE DLL | Host OS loads complete images, dependencies, TLS, and initialization |
| PE EXE, ELF EXEC/PIE, Mach-O EXEC | Identified and rejected as library inputs |
| Thin archives, fat Mach-O, COFF import libraries/bigobj | Rejected or unsupported; use an ordinary target slice/object, or load the DLL directly |
| OMF, D `.ddl`, Go gc `.a`, LLVM bitcode, raw LLVM IR | Not directly loaded; matching toolchains can first compile IR/bitcode to native `.o` |

Implemented relocation families:

- ELF x86-64: NONE, 64, PC32, PLT32, GOTPCREL/GOTPCRELX/REX_GOTPCRELX, 32/32S, PC64.
- ELF AArch64: NONE, ABS64/ABS32, PREL64/PREL32, ADRP page, ADD/LDST8/16/32/64/128 low12, CALL26/JUMP26, GOT page/load64.
- Mach-O x86-64: UNSIGNED, SIGNED, BRANCH, GOT_LOAD/GOT, external SUBTRACTOR+UNSIGNED, SIGNED_1/2/4.
- Mach-O arm64: UNSIGNED, external SUBTRACTOR+UNSIGNED, BRANCH26, PAGE21/PAGEOFF12, GOT page/offset, POINTER_TO_GOT, paired ADDEND. Local section-ordinal instruction relocations remain unsupported.
- COFF AMD64: ABSOLUTE, ADDR64, ADDR32, ADDR32NB, REL32 through REL32_5. `__imp_` names use GOT slots for explicit DLL symbols. SECTION/SECREL, weak external auxiliary aliases, and COMDAT are unimplemented.

Unknown relocations, overflow, writes outside sections, incompatible targets, and duplicate strong symbols fail without publishing a partial executable image. Exception metadata is skipped without registering an unwinder: the raw object path therefore does not support throwing exceptions or stack unwinding. Objects with recognized TLS, automatic constructors/destructors, COMDAT, or Swift/ObjC registration requirements are rejected.

## Language outputs

File format support alone does not establish language support. A callable entry also requires compatible ISA/OS ABI, an exact prototype, and an initialized language runtime.

| Producer | Boundary | Evidence and limits |
| --- | --- | --- |
| C (Clang; native GCC output may also work) | Ordinary C ABI; `-fPIC` where appropriate | Clang objects, data, BSS, common, cross-object calls, and libraries tested; compiler-specific GCC extensions not comprehensively verified |
| Assembly (llvm-mc/Clang/as) | C ABI entry points | ISA and relocation subset must match; arbitrary assembly packages are not automatically compatible |
| C++ | `extern "C"` facade or simple functions with exact mangled names | Samples without exceptions/RTTI/complex runtimes tested; complete STL, class lifetime, and inheritance need libraries and adapters |
| Rust | `extern "C"`, stable exported names, `panic=abort` | `no_std` leaf objects on both Linux/macOS architectures; Rust ABI, trait objects, and panic unwinding unsupported |
| Zig | `export fn`, C-compatible arguments | Objects on both Linux/macOS architectures; internal Zig ABI and complex layouts are not adapted |
| Fortran | `bind(C)`, `iso_c_binding`, explicit `value` arguments | gfortran objects on both Linux/macOS architectures; I/O/descriptors need runtime libraries and wrappers |
| Go gc | `go build -buildmode=c-shared`, `//export` | Libraries on both architectures of Linux/macOS/Windows; Go `.a`, ordinary Go ABI, and GC-managed layouts are not C ABI inputs |
| llgo | Compile the host with llgo; plugins export C ABI / c-shared | Both host and producer tested; LLVM output still requires correct GC, initialization, and metadata |
| Swift | Complete library with C exports | C-exported dylibs on both macOS architectures; raw registration objects rejected; native generics/async/throws unsupported |
| Objective-C / ObjC++ | OS-loaded framework/dylib with C facade or explicit adapter | Raw registration sections rejected; no objc_msgSend signature or ARC management |
| D | Independent C ABI boundary can be attempted as native input | No DDL-specific D compatibility or D compiler/runtime verification |
| Odin, Nim, Pascal, other native compilers | C exports with complete runtimes or standalone computation objects | Possible extensions, but no execution evidence and no current support claim |

`Resolve` and `Symbol.WithAddress` support caller-defined adapters for any compatible native signature or data layout. llgo can compile known C signatures into direct calls; ordinary Go can use typed cgo bridges. The loader does not infer or validate those signatures. Optional dynamic `Bind` uses libffi for `void/i32/u32/i64/u64/f32/f64/pointer`, at most 32 fixed arguments, and the host's default C ABI. It does not supply C++/Swift or cross-OS ABI adaptation.

## Other execution domains

Python/Ruby/Lua extensions, JNI, databases, and native image/audio/video plugins may reuse the loader when the host language runtime is already initialized and the entry protocol is correct. Symbol discovery does not initialize an interpreter or manage its objects.

Wasm needs a separate Go executor interface, such as a future evaluation of wazero. CUDA/HIP/Metal/OpenCL need device drivers, modules, and launch APIs. eBPF needs a kernel verifier/loader. These are candidates for independent plugin backends, and none is integrated here.

Cross-CPU execution can use a separate process with RPC; cross-OS execution additionally requires the relevant system and runtime. Relocation cannot replace emulation, Wine, ABI thunks, or OS services.
