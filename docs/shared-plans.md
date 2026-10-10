# Shared native call resources

`Session.Bind` maintains two session-owned indexes. Equal logical signatures
reuse the same `abi.CallPlan` across symbol addresses. Distinct logical
signatures with an equal `Signature.ABIShape()` retain independent call plans
and share their native CIF, prepared layouts and bounded idle buffer pools.
The indexes contain no code addresses and never cross session lifetimes.

| Signature difference | Logical plan | Native resources |
| --- | --- | --- |
| Omitted versus explicit scalar metadata | Same | Same |
| Struct field names, including nested records/arrays | Separate | Shared |
| Pointer element descriptions | Separate | Shared |
| `Default` versus `CDecl` | Separate | Shared |
| Variadic tail types with equal C promotions, such as `float32`/`float64` | Separate | Shared |
| Fixed scalar kinds or signedness | Separate | Separate |
| Aggregate member order/types or array lengths | Separate | Separate |
| Variadic status/fixed-prefix length or other conventions | Separate | Separate |

This is a conservative comparison for the current C backend on one host.
`ABIShape` validates before removing metadata and returns an independent
snapshot. It does not describe register allocation, establish compatibility
between language ABIs, or provide a persistent/cross-platform identifier.
Keeping scalar kinds and aggregate structure avoids assuming that similarly
sized types have interchangeable native calling conventions.

Each logical plan still validates its original values, applies its own
variadic promotions, enforces declared temporary pointee types and reconstructs
its own result field names. Declared pointee layouts are prepared lazily;
a failed oversized copy cannot invalidate another logical plan. Raw pointer
values remain opaque. Native calls never receive raw Go aggregate memory.

For independently managed addresses, `abi.Prepare` creates the first plan and
`CallPlan.Share(signature)` creates an independently closed logical plan. The
source plan must be open and the signatures must have equal ABI shapes. A
surviving shared plan can create another share after the original closes.
Each plan rejects new calls during its own retirement and waits for its active
calls. The last plan releases native resources after those calls return.
Plans do not own target code; callers must retain its image separately.

Calls can overlap and reenter through different logical plans. Each active call
borrows exclusive native storage; idle pool limits apply to the entire shared
resource group. No sharing lock is held during native execution or callbacks.
Applications must still synchronize mutable pointees and native state.

Failed symbol lookup closes a newly prepared/shared plan without publishing
either cache entry. `Session.Close` retires all session-owned logical plans and
releases each native resource group once. Native tests cover objects, archives,
OS libraries, concurrent record calls, cross-plan callback reentry, independent
result metadata, pointer contracts, failure isolation and closing order.
