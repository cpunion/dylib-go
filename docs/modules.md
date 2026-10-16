# Versioned module dependencies

The optional Go-only `module` package loads explicit dependency manifests using
ordinary `dylib.Session` owners. It adds graph planning and exact version checks;
native parsing, archive selection, linking and code retention stay in the core.
No llgo compiler, libffi or external dependency resolver is required.

| API | Behavior |
| --- | --- |
| `Manifest.Validate()` | Check structural contracts, names, versions, declared exports and cycles without reading native inputs |
| `Read(reader)` | Decode and validate one JSON document, up to 1 MiB; reject unknown fields and trailing documents |
| `Open(base, manifest, options)` | Load selected roots and their transitive providers, resolving relative file paths from `base` |
| `Load(path, options)` | Read a manifest and use its containing directory as the input base |
| `Graph.Session(name)` | Borrow one loaded session for normal library calls and adapters |
| `Graph.Close()` | Retire graph lookup, then close consumers before providers; concurrent closes share the cleanup result |

The [executed manifest example](../examples/modules/manifest.go) imports a C
function and global data using different consumer linkage names. Native tests
serialize that same manifest to JSON, load it from a separate directory, and
execute the consumer through a typed Go or direct llgo adapter without libffi.

## Manifest contract

| Field | Meaning |
| --- | --- |
| `format` | Required schema version, currently `module.FormatVersion` (1) |
| `roots` | Distinct selected module names; at least one |
| `modules[].name` | Unique, case-sensitive module identifier |
| `modules[].version` | Nonempty opaque ABI contract version |
| `modules[].files` | One or more explicit native input paths; objects, archives and complete OS libraries use normal content identification |
| `modules[].exports` | Exact core linkage names made available to consumers; also explicit roots for lazy archive extraction |
| `modules[].imports` | Explicit provider, exact required version, exported symbol, and optional consumer alias `as` |

Providers can appear after consumers in the document. Root and import order
determine a stable dependency traversal; a shared provider is loaded once even
when multiple roots or imports use it. Unreachable definitions are validated but
their files are not opened. Unused archive members remain unselected; declaring
an export explicitly makes it an archive root.

Versions match exactly. They are supplied contracts, not a binary ABI probe,
semantic-version range, signature inference or authenticity check. Missing
providers, version mismatches, undeclared imports, duplicate aliases and cycles
fail before native file loading. Actual exports, reserved definition names,
relocations, calling conventions and host compatibility still follow core
validation. A raw missing export root fails before initialization. OS libraries
can initialize as soon as they are loaded, before later failures are discovered.

`DefineSymbol` supplies retained function/data addresses to each consumer. Normal
core name precedence applies. This does not rewrite an OS library's internal
imports or adapt an ABI: complete libraries must already have their normal
native runtime/dependency linkage. `dylib.Options` apply to every session without
rebasing their `LibraryPaths`; file paths alone use the manifest base directory.
Absolute input paths remain absolute. Keep the manifest/options stable during
`Open`.

## Lifetime and failure

The graph owns its sessions. Borrow them for `Resolve`, `Bind`, or application
adapters; call `Graph.Close` to close the whole graph. Closing an individual
provider first can wait for still-open consumers. Returned symbols/functions and
their leases retain code under the existing core rules.

`Close` first rejects new graph lookups, then closes sessions in reverse
dependency order. A provider remains alive through consumer native finalization.
External leases can delay cleanup; release them after their native users finish.
Stop native registrations, join workers and destroy language objects before
closing their graph. Call close outside its own calls, callbacks and finalizers.

Open failure closes every session created by that attempt, including the failing
consumer. Cleanup releases retained provider leases in dependency order and
returns any cleanup errors together with the original failure. Native
initialization/finalization has side effects; cleanup does not undo those effects.

CI runs object/archive/library providers and multiple roots at `-O0` and `-O2`,
including archive members that must remain unused. Lifecycle tests observe native
initializers/finalizers, a failed consumer link, stale exports, exact-version
preflight, and concurrent graph close while an external observer lease remains
callable. These execute on all eight Go targets and six llgo targets. No new
execution platform or object format is introduced by this package.

Filesystem/provider discovery, version negotiation, module replacement and state
migration remain separate work.
