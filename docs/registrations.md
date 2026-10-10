# Retained native registrations

`NewRegistration(resources, stop)` groups independent leases on explicitly
declared code, native storage and callbacks. It requires no backend by itself:
symbols and caller-defined adapters work without libffi. Bound `Function`,
`abi.NativeValue` and `abi.Callback` resources use their existing optional libffi
backend. Go and llgo share the same ownership implementation.

The [library example](../examples/registration/main.go) lets C retain a record and
a Go callback across separate calls. The existing example runner builds and runs
it with both compilers: `examples/run.sh go library` or
`examples/run.sh llgo library`.

## Construction and use

1. Resolve/bind native entry points and create values/callbacks. Include every
   provider image needed by the registration and its stop function.
2. Construct `RegistrationResources` with `Symbols`, `Functions`, `Values` and
   `Callbacks` lists. `NewRegistration` acquires leases in that order; acquisition
   failure releases earlier leases before returning, without calling stop.
3. Publish addresses inside `Registration.WithLeases`. `RegistrationLeases`
   contains borrowed leases in the same order as each resource list. Slice
   snapshots are independent; do not close the leases yourself.
4. Keep the registration until native access has ended. `WithLeases` can perform
   setup and subsequent calls, concurrently or recursively when the C API permits
   it. A use error or panic releases that operation's guard, but leaves the
   registration retained; call `Close` to clean up partial setup.
5. Call `Registration.Close` outside its own operations, callbacks and stop
   function. Close waits for synchronous operations, then calls stop with all
   resource leases alive. Stop must unregister addresses and join native users,
   returning nil only after no access remains. It must also handle absent or
   incomplete registration, because Close calls it even before any setup ran.

Resource owners are independent: registration cleanup releases its leases, not
the Session, NativeValue or Callback itself. The application closes those owners
when they are no longer shared. Callback lists cannot contain the same owner
twice; this avoids reacquiring a callback read lease behind a pending close.
There is no finalizer and no automatic inference of native dependencies.

## Code leases and retirement

`Symbol.Acquire` produces a `SymbolLease`; `Address` exposes a retained native
address, and `WithAddress` guards individual adapter calls. `Function.Acquire`
produces a `FunctionLease` with `Call` for the already bound signature. Both keep
the whole session image, libraries and session-owned plans alive. They support
overlapping and reentrant calls without holding an invocation mutex.

`Session.Close` retires normal lookups, calls and acquisitions immediately, then
waits for active users and leases. Existing leases remain usable during this
wait: a `FunctionLease` can execute native unregister/join even though the
original `Function.Call` returns `ErrClosed`. Closing a code lease rejects new
lease calls, waits for active calls and releases its session retention once.
Raw retained address users must be stopped before releasing that lease.

Do not synchronously close a resource owner while holding its lease on the same
path: close waits for that lease. Close registrations first, then their owners,
or let another goroutine retire the owner while registration cleanup runs.

## Stop failure and retry

| Registration state | New WithLeases operations | Native resources | Close |
| --- | --- | --- | --- |
| Open | Accepted | Retained | Starts retirement |
| Retiring | `ErrClosed` | Retained through unregister/join | Concurrent callers share the current attempt |
| Stop failed | `ErrClosed` | Still retained | A later call retries stop |
| Closed | `ErrClosed` | Leases released | Idempotent |

A stop error is returned unchanged. Concurrent callers waiting on one attempt
receive that attempt's result. A panic is preserved; waiting closers receive an
incomplete-stop error, all leases remain retained, and a later Close may retry.
The stop function must account for any partial work it already completed.

Keep the registration handle on cleanup failure. Do not wait on resource-owner
Close before retrying: those owners still have leases intentionally retained.
The owner cannot revoke addresses kept by uncooperative native code or discover
unlisted dependencies. Native memory synchronization, language destructors and
exception boundaries remain the application's ABI contract.

## Execution coverage

The existing platform suites execute real C-retained record/callback calls through
objects, archives and OS libraries with Go and llgo. Tests retire the session,
value and callback owners, reject ordinary calls, retry failed cleanup and invoke
the retained C entry during unregister. A separate shared library starts a native
OS thread: registration cleanup joins that thread while its Go handler reads
leased storage, then releases all owners. Pure Go tests cover acquisition
rollback, independent leases, concurrent/reentrant use, close ordering, errors
and panic cleanup. No extra workflow-specific test commands are needed.
