# Retained symbol dependencies

`Session.DefineSymbol(name, symbol)` imports a resolved function or data symbol
from another session. It acquires its own `SymbolLease`, registers the requested
linkage name through the normal definition rules and retains the provider until
the consumer has returned from native image and library cleanup. Import aliases
may use a different name; this does not infer or adapt a native signature.

| Operation | Code ownership |
| --- | --- |
| `Define(name, address)` | Caller retains the native address owner |
| `DefineSymbol(name, symbol)` | Consumer retains the symbol's provider session |
| Consumer call or code lease | Transitively retains all accepted symbol dependencies |
| Consumer `Close` | Waits for uses, runs native cleanup, then releases dependency leases |
| Provider `Close` | Rejects new acquisitions and waits for dependent consumers |

Resolve the provider first, add retained definitions before linking the consumer,
then use normal `Resolve`, `Bind` or caller-defined adapters. Resolving or binding
a provider seals it under the existing two-phase session model. It cannot acquire
new definitions after publication, so this API cannot create a cycle of retained
session dependencies. Importing a session's own symbol is rejected.

Close consumers before providers, or close both concurrently. Calling provider
`Close` first in the same goroutine waits for still-open consumers. Existing
consumers and their code leases can continue invoking retained provider code
during provider retirement. A closed provider cannot supply new dependencies.
Releasing the consumer does not call provider `Close`; the provider remains an
independent owner and may serve other consumers.

All normal definition validation still applies: null addresses, invalid names,
duplicates, reserved runtime names and definitions after linking are rejected.
A failed definition releases its newly acquired lease. A failed link retains
accepted dependencies so the consumer can add missing inputs and retry. If a
native initializer fails, dependency code stays alive for the failed image's
registered exit callbacks until consumer cleanup. Repeated/concurrent consumer
close releases each acquired lease once.

The core API uses Go ownership and does not require libffi. Call signatures,
mutable native data synchronization, retained callback registrations, TLS and
language runtime requirements remain caller responsibilities. Dependency
discovery, manifests, version selection and module replacement are separate work.

The [library example](../examples/dependencies/main.go) imports a C function and
global variable from a provider object. Run it with
`examples/run.sh <go|llgo> dependencies <provider> <consumer>`; the `library`
example suite also builds both C fixtures and checks the result in CI. Native
tests use raw objects, archives and OS-library providers, execute imported
functions/data while both owners retire, and observe finalizer/failed-initializer
cleanup before provider release.
