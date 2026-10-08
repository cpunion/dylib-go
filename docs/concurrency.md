# Concurrent calls and owner retirement

`Symbol.WithAddress`, `Function.Call`, and `abi.CallPlan.Call` may overlap or
reenter the same owner. A Go callback can resolve/bind another function in its
calling session or recursively call the same bound Function. The owner mutex
protects state changes and active-use counts; it is released before invoking
adapters/native code.

Prepared libffi CIFs and layouts are immutable after preparation. Calls have
separate argument/result buffers and temporary native pointee copies. This
follows [libffi's thread-safety rules](https://github.com/libffi/libffi/blob/v3.4.8/doc/libffi.texi#thread-safety):
prepare each owned type graph before publication, then share its prepared CIF.
Concurrent callers must still synchronize mutable Go pointees, callback
captures, and native state that their C API does not permit sharing.

## Close and retirement

| Owner state | New calls/lookups | Existing calls | Cleanup |
| --- | --- | --- | --- |
| Open | Accepted | May overlap or nest | None |
| Retiring after Close starts | `ErrClosed` | Retain code, libraries, and call layouts until return | Waits without an invocation lock |
| Closed | `ErrClosed` | None | Completed |

`Session.Close` retires the owner before waiting. A callback racing close can
query the session and receive `dylib.ErrClosed` immediately. It cannot start a
new call while the outer native invocation remains active. Existing uses finish
normally, including result conversion/copy-back, before unmapping or DLL release.
`CallPlan.Close` applies the same protocol to its prepared ABI resources and
returns `abi.ErrClosed` for new calls. Concurrent close callers wait for the
same cleanup; cleanup runs once. A panic in a Go address adapter releases its
active-use guard while preserving the panic.

Session finalizers run outside the session mutex after retirement. They may
observe `ErrClosed` from session methods without a lock cycle. Call `Close`
outside an owner's own adapters, native handlers, or finalizers: close waits
for those operations and cannot wait for itself. Other resource owners retain
their own contracts; in particular, do not reacquire/close a callback lease
from its handler when a callback close could be pending.

## Boundaries

These synchronous guards do not own arbitrary pointers retained by native
code. Join workers and remove native registrations before releasing addresses
or callback leases. Raw `Lookup` addresses require caller-managed lifetimes.
Loading and linking remain serialized staging operations; constructors during
OS load or raw initialization must not reenter the loading session. Supporting
staged-symbol lookup during initialization requires a separate linking state.

CI executes same-Function recursion for objects, archives, and OS libraries,
simultaneous native calls, independent struct-pointer copies, close during a
callback, and finalizer queries. A C library also starts four native threads,
each performing C → Go → same-session C → Go calls through a shared plan. The
existing Go and llgo platform suites execute these tests, including Go 386.
