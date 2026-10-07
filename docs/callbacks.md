# Native C callbacks

`abi.NewCallback(signature, handler)` creates a native C function pointer for a Go closure. It requires cgo, `-tags libffi`, and a libffi build with closure support. Ordinary Go's default build keeps the API available but returns `abi.ErrUnavailable`; enabling dynamic calls does not force applications to create callbacks. The independent callback owner can be used with raw objects, archives, OS libraries, or caller-managed native APIs.

## Signature and values

The signature is a deep copy of `abi.Signature`. Fixed host C ABI / cdecl entries support signed and unsigned 8/16/32/64-bit integers, C bool, float32/64, pointers, void results, and ordinary struct arguments/results. Windows 386 also supports explicit stdcall and fastcall. There are at most 32 arguments; records use the same field, nesting, and size limits as dynamic calls. Variadic callbacks, arrays, unions, packed records, bitfields, and native C++/Swift closures are unsupported.

Handlers receive fresh logical `[]abi.Value` arguments. Struct fields are decoded using the prepared native layout, rather than casting Go memory to a C struct. A pointer argument contains a borrowed native address in `Bits`; it is not automatically dereferenced or converted into a `Pointee`. Native storage and its lifetime remain the caller's responsibility. Results may contain caller-owned native addresses, but `abi.AddressOf` and any temporary pointee nested inside a result are rejected: their storage cannot outlive this entry. Go heap addresses must not be published as native retained pointers.

Native callers must use the exact declared prototype and convention. The callback marshaler cannot discover or repair an incompatible call. libffi classifies struct/register/hidden-result conventions and allocates the callable entry; its [closure API](https://github.com/libffi/libffi/blob/master/doc/libffi.texi) requires a live CIF/type graph until the closure is freed. Small integer results are widened to `ffi_arg`/`ffi_sarg` as required by that API. Executable closure memory comes from libffi's allocator, independently of the raw object image's W^X mapping.

## Ownership and retirement

The callback owns its native entry, CIF/type graph, and an opaque `runtime/cgo.Handle` rooting the Go closure and captures. There is no finalizer that frees executable code during GC. Call `Callback.Close` explicitly.

For a synchronous call, use `Callback.WithAddress`; its lease lasts until the supplied function returns. Native workers must finish and native registrations must be removed before that return. The [README example](../examples/callback/main.go) demonstrates this contract.

For a stored registration:

1. Create the callback and acquire a `CallbackLease`.
2. Obtain `lease.Address()` and register it with native code.
3. Keep the lease and the registering library/object image alive while callers can use the entry.
4. Unregister the native entry and wait for all in-flight callers or native workers to finish.
5. Release the lease, then close the callback and the library/session as appropriate.

`Callback.Close` waits for every lease and frees the native entry, plan, and capture handle afterward. Lease close and callback close are idempotent; closed acquisition/address access returns `abi.ErrCallbackClosed`. A lease guards ownership, not native use-after-free: releasing it while native code still holds or executes the address violates the contract. Neither the callback nor `Session.Define` automatically owns a native registration or a target session.

Do not acquire another lease or close the callback from its handler or from `WithAddress`: a pending close can cause a lock cycle. Do not re-enter locking methods of the session currently invoking the callback. For example, calling a function on that same session from its callback blocks on the session's existing call lock. Applications needing such operations must design separate ownership and call boundaries.

## Errors and threads

A handler error, recovered panic, or invalid result leaves a zero native result, including a zero-filled struct or null pointer. `Callback.Err()` retains the first failure and remains available after close; successful calls do not clear it. There is no automatic C error-code convention. Applications can return explicit C-compatible error codes through their signature instead. `runtime.Goexit`, process exit, native signals, exceptions, and `longjmp` are outside the recovery contract; native code and handlers must return normally.

The native entry may be called concurrently, including from C-created OS threads. Each invocation has separate argument values; user captures and pointed-to native data need synchronization when shared. Ordinary Go's exported cgo entry handles transitions from foreign threads.

The llgo backend enables foreign-thread registration before publishing callbacks and calls the runtime's enter hook before manipulating Go values. Collector registration is retained until the foreign thread's native lifecycle ends; the corresponding exit hook follows the runtime's teardown contract. These isolated `go:linkname` hooks are verified against [llgo v1.0.6](https://github.com/xgo-dev/llgo/blob/v1.0.6/runtime/internal/runtime/foreign_thread_gc.go). They are internal runtime interfaces, so a future llgo version requires revalidation. This adapter does not add TLS relocation or thread-local storage support to raw objects.

## Execution tests

CI runs the callback fixtures with ordinary Go on Linux/Windows/macOS amd64 and arm64, plus Linux/Windows 386, and with llgo on the six amd64/arm64 hosts. Tests cover objects, archives, and OS libraries; all scalar widths; zero arguments and void; pointers; nested, large, and floating-point structs; panic/error/invalid-result handling; captures; retained registration and lease retirement; and Windows 386 stdcall/fastcall. A separate C library starts native threads that invoke one captured handler concurrently, allocate Go values, trigger GC, and join before lease release. The README example is compiled and executed by those same jobs.
