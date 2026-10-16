# Replacing module graphs

`module.Manager` owns a current graph and publishes explicit replacements while
retaining old calls and code leases. Its implementation uses Go synchronization
and the existing graph/session ownership rules. It introduces no ABI adapter,
runtime linkname or libffi requirement.

| API | Behavior |
| --- | --- |
| `NewManager(graph)` | Take ownership of an open graph on success |
| `WithGraph(use)` | Borrow one generation until the callback returns or panics |
| `Replace(build, migrate)` | Build a candidate, optionally migrate state, publish it, then wait for old uses and cleanup |
| `Close()` | Immediately retire admission/publication, then wait for replacement/scopes/leases and close the current graph |

The [executed state migration example](../examples/modules/replace.go) copies a
scalar through native `graph_state`/`graph_set` accessors. Both use the fixture's
known `int32_t(int32_t,int32_t)` ABI with Go cgo or direct llgo calls. Applications
provide their own state accessors and call adapters. Dynamic `Bind` and generated
C/C++ bindings can be used inside these scopes too.

## Publication and state

The builder typically calls `module.Open` or `module.Load`. It must return a fresh,
unowned, open graph; returning the current graph is rejected without closing it.
Each non-nil returned candidate transfers to the manager, including when the
builder also returns an error. The builder remains responsible for resources it
has not returned, including if it panics before returning.

Migration borrows the old and candidate graphs while both remain alive. Ordinary
old calls may continue during build and migration. The caller must synchronize
mutable native state and stop/join or transfer native registrations as its API
requires. The manager holds no admission mutex across build, migration, native
calls or cleanup, so `WithGraph` reentry is allowed until close retires admission.
Migration is not an implicit quiescence barrier or a native state transaction.
Initialization and migration effects cannot be undone automatically.

After successful migration, publication changes the graph used by new scopes.
Existing scopes retain their original generation. Replacement waits for those
scopes to return and then closes the old graph, respecting external core code
leases. New scopes can execute against the replacement during this wait.

| Result | Current selection |
| --- | --- |
| `published == false`, non-nil error | The candidate was not installed; any returned candidate is closed |
| Panic after builder return, before publication | The candidate is closed; the panic propagates and the previous selection remains |
| `published == true`, nil error | The replacement was installed and old cleanup completed |
| `published == true`, non-nil error | The replacement was installed; old cleanup reported an error |

Concurrent close may already have retired a newly published graph before
`Replace` returns. The boolean records publication, not a promise that no later
operation occurred. Closed managers reject replacement before invoking its builder.

## Ownership and close

Graphs and sessions passed into scopes/migration are borrowed; do not close or
retain those borrowed owners independently. Acquire a symbol/function lease for
native code that must outlive a scope. Existing bindings, pointers, language
objects, callbacks and registrations retain their original ABI and lifetime;
publication does not rewrite them or move their state. Stop native users and
release their old leases so replacement can finish retirement. Calling replacement
in the same goroutine while holding an old lease would wait for that lease.

For complete libraries, normal OS loader caching still applies. Use distinct
native library identities/paths when a new binary image is required; replacing a
manifest does not force the OS to reload the same library reference. Raw object
sessions continue to own independent snapshots and mappings.

Close retires new admission immediately, even while a builder or migration is
running. That in-progress replacement closes its candidate without publishing
when it observes retirement. Concurrent closes share one result. Do not call
this manager's `Replace` or `Close` from its own scope, builder, migration, native
callback or finalizer: draining that operation would wait for itself.

Tests run real old/new native code, migrate state, verify failure/panic cleanup,
publish while old scopes and external leases remain callable, and close during
build/migration or old retirement. Native jobs execute these on all eight Go
targets and six llgo targets, without libffi.
