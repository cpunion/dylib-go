# Optional native Swift calls

`abibridge.Runtime` prepares explicit native Swift signatures and invokes thin
function symbols from OS-loaded dylibs. This is a separate Swift ABI backend;
`Session.Bind` and C signature strings keep their existing C ABI meaning.

| Contract | Support |
| --- | --- |
| Host and function images | macOS 15.4+, amd64/arm64; complete native dylibs |
| Go / llgo transport | Fixed C facade entries through cgo; no Go `libffi` build tag |
| Scalars | Signed/unsigned 8/16/32/64-bit integers, Float/Double, native addresses, void results |
| Trivial Swift values | Explicit compiler-confirmed sizes, alignment and scalar component offsets; supported natural component layouts |
| Large values / results | Backend-selected indirect storage based on the declared Swift components |
| Formally indirect values | Explicit `Indirect` storage; tested with a small resilient Swift struct |
| Concurrency | Reusable interfaces, independent per-call storage, function-image leases, close waits for active invocations |
| Unsupported | Reference-bearing values, String/Array ownership, managed closures, tuples/packs, inout/consumed parameters, generics, async/throws, self contexts, Objective-C dispatch, UI threading, arm64e and devices |

## Build and load

Supply clean local checkouts of [ABIBridge
`4dfbda2`](https://github.com/lynnswap/ABIBridge/tree/4dfbda22afb9a1492e1c5c99933a9defbc5437a4)
and its [ZDLibffi
`e9737c7`](https://github.com/lynnswap/ZDLibffi/tree/e9737c755088a4a3d2ba03f2ca5bfeadff9c09b2)
to `compiler/abibridge.Build`. It checks both revisions and local changes, then
uses the selected Xcode tools. It does not fetch sources or select another SDK.
Keep tool and SDK selection consistent when supplying custom environment flags.

The builder creates a private tools-5.9 SwiftPM manifest for the C++20 native
invocation sources and libffi only. It does not compile SDK-specific pointer
hooks or build ABIBridge's Swift runtime/UI products, and leaves upstream
manifests unchanged. Individual objects are archived before linking. Four private C exports implement
version checking, interface creation, invocation and release. Upstream APIs and
libffi symbols are hidden from this facade's exports.

Open the resulting `Library.Path` with `abibridge.Open`. Close every runtime
before closing its build artifact. The upstream projects retain their own MIT
licenses; preserve their copyright/license notices when distributing compiled
facades. The project's core loader has no default ABIBridge dependency.

The executable [Go library example](../examples/swiftcall/call.go) prepares an
Int32 signature. The same `Prepare` and `Invoke` methods support all rows above;
the [native tests](../abibridge/native_test.go) include mixed integer/floating
records, tail sizes different from C struct sizes, padded results, forty-byte
values, formally indirect storage and twenty parameters exhausting both register
banks. Compiler-emitted IR confirms these are `swiftcc` functions.

## Storage and lifetime

`Type` describes the Swift function's actual storage and formal passing mode.
For a scalar, set `Kind`. For `Storage`, supply size, alignment and scalar
components with verified offsets. Components must have supported natural
aggregate offsets, but the Swift value size need not include C tail padding.
Use `Indirect` only when the actual Swift signature is formally indirect.
There is no automatic C-to-Swift record conversion or runtime type inference.

`Invoke` accepts one byte slice per argument and returns independent result
bytes. All supported hosts are little endian. Descriptions are snapshotted when
prepared. The native facade copies inputs into aligned, zeroed native storage,
dispatches through ABIBridge and copies only the declared result size. Storage
allocation includes aligned stride. Result padding starts zeroed.

Descriptions allow at most 256 arguments, 64 scalar components per value, 1 MiB
per value, 4096-byte alignment and 16 MiB total call storage. Both Go and the
native decoder check bounds. Input byte counts must match exactly.

Bytes must represent trivial values with guaranteed parameter ownership.
Reference-bearing Swift values require metadata and copy/destroy witnesses;
their representation bytes are not an ownership transfer. Native pointer values
need separate data/code leases for as long as the pointees can be used.

Swift runtime metadata can outlive a call or session. Load providers with
`Options{KeepLibraries: true}`, as for the existing Apple C facades. `Invoke`
additionally leases the selected symbol for the active call; runtime/interface
close waits for those calls before releasing its native plan. Closing a runtime
or interface from its own invocation is not supported.

## Validation

The macOS Go and llgo CI jobs fetch the two immutable source revisions, build one
facade per job, then run native tests and the real README example. Missing
configured dependencies fail. Go also executes race/lifetime checks. Native
tests run unoptimized/optimized Swift code, narrow and wide scalar boundaries,
floating values, record layouts, pointer allocation/mutation/free, reusable and
concurrent interfaces, malformed native descriptions and close during a blocked
Swift invocation. No-libffi local runs use the same C transport.

Pure Go tests validate metadata and unsupported-host behavior on the full project
matrix. Cross compiling the core or parsing an Apple image does not establish
device, arm64e, or other-OS Swift execution support.
