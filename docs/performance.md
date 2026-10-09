# Measuring dynamic calls

Use a bound `Function` for repeated calls into a session. `abi.Prepare` owns a
reusable plan when an adapter manages code lifetime separately. `abi.Call`
prepares and frees a plan for every call; it is convenient for occasional use.

| Benchmark mode | Work inside the measured loop | Code lifetime |
| --- | --- | --- |
| Bound | Argument validation, marshaling and native call | Session guard for each call |
| Prepared | Argument validation, marshaling and native call | One surrounding Symbol guard |
| OneShot | Signature copies, CIF/type construction, validation, marshaling, call and cleanup | One surrounding Symbol guard |

Run `go test -tags libffi -run '^$' -bench '^BenchmarkNativeCalls$' -benchmem .`
or replace `go` with `llgo`. Use `-bench '^BenchmarkNative(Calls|BindingCache(Size)?)$'` to include warm session binding and cache-size comparisons. The benchmarks use the same compiled C fixture and
check every result. Compilation, load and initial binding are outside the
measured loops. Cases cover two scalars, 64 scalars, a by-value struct, and a
temporary struct pointer with copy-back. The 64-argument function also performs
more native arithmetic, so differences between shapes are not pure FFI overhead.

Keep the machine otherwise idle and record toolchain versions and CPU identity.
Use multiple runs for performance decisions. `B/op` and `allocs/op` measure only
allocations exposed by the selected Go runtime; they exclude native
`malloc`/`calloc`, libffi storage and image mappings. Allocation counts from
different runtimes need separate profiling before direct comparison.

## Local reference measurement

These are medians of three 200 ms runs on an Apple M4 Max, darwin/arm64,
recorded after adding session/type-layout caching on 2026-10-08 with Go 1.27.0, llgo `b86d349178d6` and libffi 3.8.0.
They describe this dynamic libffi path, including logical value handling and
owner guards; they do not benchmark llgo's direct typed C adapters.

| Shape | Mode | Go ns/op | llgo ns/op | Go B/op | Go allocs/op |
| --- | --- | ---: | ---: | ---: | ---: |
| Scalar2 | Bound | 87.0 | 310.3 | 24 | 2 |
| Scalar2 | Prepared | 81.4 | 226.2 | 24 | 2 |
| Scalar2 | OneShot | 524.2 | 985.5 | 640 | 11 |
| Scalar64 | Bound | 962.6 | 1610.0 | 520 | 2 |
| Scalar64 | Prepared | 948.4 | 1479.0 | 520 | 2 |
| Scalar64 | OneShot | 5622.0 | 8914.0 | 6208 | 78 |
| Struct | Bound | 632.6 | 1421.0 | 168 | 6 |
| Struct | Prepared | 611.9 | 1306.0 | 168 | 6 |
| Struct | OneShot | 1625.0 | 2868.0 | 1432 | 27 |
| Pointer | Bound | 1412.0 | 3057.0 | 1576 | 17 |
| Pointer | Prepared | 1379.0 | 2935.0 | 1576 | 17 |
| Pointer | OneShot | 2607.0 | 4413.0 | 2904 | 39 |
| Warm binding | Signature cache hit | 240.7 | 922.7 | 192 | 4 |

The measured llgo runtime reports `0 allocs/op` for every case despite nonzero
`B/op`; that counter is not evidence of allocation-free execution. Reproduce
measurements on the application host before drawing performance conclusions.

## Indexed binding comparison

The following medians compare the previous linear signature cache with the
indexed cache on the same host/toolchains on 2026-10-09. Each size case prepares
that many distinct typed-pointer signatures, then repeatedly binds the last
one. Compilation and initial preparation are outside the measured loop. These
are three 200 ms samples per case; they measure warm `Bind`, not native call
latency or cold preparation. Run
`go test -tags libffi -run '^$' -bench '^BenchmarkNativeBindingCache(Size)?$' -benchmem -benchtime=200ms -count=3 .`
and repeat with `llgo`.

| Cached signatures | Go before ns/op | Go indexed ns/op | llgo before ns/op | llgo indexed ns/op | Go allocations before → indexed |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 480.1 | 183.1 | 2483 | 712.1 | 4 → 3 |
| 16 | 6178 | 181.8 | 34928 | 706.3 | 34 → 3 |
| 256 | 108247 | 182.3 | 554351 | 736.6 | 514 → 3 |
| 4096 | 1810061 | 178.5 | 8497625 | 709.0 | 8194 → 3 |

The separate two-scalar warm-binding case changed from 319.9 to 173.0 ns/op
under Go, and from 1202 to 537.4 ns/op under llgo. Indexed cases use 48 Go B/op;
the remaining allocations include the returned `Function` and `Symbol` plus
the signature key. Larger descriptors still require validation and encoding.
This removes the scan through existing signatures without changing invocation
storage or merging logically different signatures.

## Aggregate buffer reuse comparison

Using the same host/toolchains on 2026-10-09, medians of three 200 ms runs compare
aggregate calls before and after bounded argument/result buffer reuse. Run
`go test -tags libffi -run '^$' -bench '^BenchmarkNativeCalls/(Struct|Pointer)/(Bound|Prepared)$' -benchmem -benchtime=200ms -count=3 .`
and repeat with `llgo`. Lazy buffer acquisition is included and amortized over
the repeated calls. Fixture compilation, loading and plan preparation remain
outside the measured loop.

| Shape / mode | Go before → reused ns/op | llgo before → reused ns/op | Go before → reused B/op | Go before → reused allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Struct / Bound | 748.9 → 346.6 | 1712 → 1122 | 168 → 0 | 6 → 0 |
| Struct / Prepared | 781.1 → 336.2 | 1689 → 1029 | 168 → 0 | 6 → 0 |
| Pointer / Bound | 1976 → 907.7 | 3757 → 2541 | 1576 → 464 | 17 → 7 |
| Pointer / Prepared | 2232 → 904.1 | 3642 → 2488 | 1576 → 464 | 17 → 7 |

These figures cover the small fixtures above. At the time of this measurement,
returned aggregates still required logical Go values and temporary pointees
still used per-call native allocation. Large contexts exceeding the idle budget
were freed rather than retained. The
scalar-only path was unchanged by that aggregate optimization. Native allocations are excluded from the Go
allocation counters; llgo's reported allocation count has the limitation
described above.

## Scalar buffer reuse comparison

On the same host/toolchains on 2026-10-09, three 200 ms runs compared the scalar
path before and after bounded native buffer reuse. Run
`go test -tags libffi -run '^$' -bench '^BenchmarkNativeCalls/Scalar(2|64)/(Bound|Prepared)$' -benchmem -benchtime=200ms -count=3 .`
and repeat with `llgo`. Acquisition is lazy and amortized over repeated calls.
Compilation, loading and plan preparation remain outside the measured loop.

| Shape / mode | Go before → reused B/op | Go before → reused allocs/op | llgo before → reused B/op (rounded) |
| --- | ---: | ---: | ---: |
| Scalar2 / Bound | 24 → 0 | 2 → 0 | 640 → 656 |
| Scalar2 / Prepared | 24 → 0 | 2 → 0 | 416 → 432 |
| Scalar64 / Bound | 520 → 0 | 2 → 0 | 1184 → 656 |
| Scalar64 / Prepared | 520 → 0 | 2 → 0 | 960 → 432 |

Go no longer allocates an argument-bit slice or escaped result word per scalar
call. Native argument packing also reuses its owned block, including above 32
arguments; the old large-call path allocated and freed packing storage each
time. Warm calls use one C transition, with cleanup in Go. Native allocation
still occurs on a cache miss, and oversized contexts are freed on return.
The measured llgo runtime retains other dynamic-call allocations, and the
small-call measurements report 16 additional bytes; its zero allocation-count
report does not mean zero allocation. Latency samples varied substantially across runs, so
this comparison reports stable allocation measurements without a latency claim.
Variadic promotion, callbacks and temporary pointees can still allocate.

## Temporary pointee buffer reuse

The pointer benchmark above copies a typed struct into native memory and copies
it back after the call. With bounded context-owned pointee buffers, repeated
calls reuse those native bytes instead of allocating/freeing them each time.
Logical aggregate construction during copy-back still allocates Go values.
On the same host/toolchains on 2026-10-09, three 200 ms samples measured:

| Pointer mode | Go before → reused B/op | Go before → reused allocs/op | llgo before → reused B/op (approximate) |
| --- | ---: | ---: | ---: |
| Bound | 464 → 464 | 7 → 7 | 5296 → 5232 |
| Prepared | 464 → 464 | 7 → 7 | 5072 → 5008 |

Reproduce with
`go test -tags libffi -run '^$' -bench '^BenchmarkNativeCalls/Pointer/(Bound|Prepared)$' -benchmem -benchtime=200ms -count=3 .`
and repeat with `llgo`. These counters exclude native value allocations and do
not quantify the removed native allocation/free operations. Buffer identity,
zeroing, budgets and cleanup are checked by tests. Cold calls, cache overflow
and ephemeral layouts for opaque pointer shapes still allocate native storage.
Latency samples were unstable, so this comparison makes no latency claim.

## Storage and cache scope

CIF argument types and aggregate layouts are owned by each prepared plan. Bindings
with the same canonical logical signature share a plan within their session;
each symbol retains its own address. A string-keyed map replaces comparisons
against every cached signature. Validation and key construction still depend on
the size of the supplied signature. Omitted and explicit scalar descriptors,
including empty metadata slices, encode identically. Field names, pointee types,
array lengths, conventions and variadic boundaries remain distinct. Invalid
metadata is rejected before lookup, and failed symbol resolution does not cache
a newly prepared plan. No resources are shared across sessions.
Scalar calls borrow an exclusive native block containing value bits, packed
arguments, a mutable address vector and result storage. The C helper returns
result bits by value, avoiding a per-call Go result allocation. Native allocation
and slice lengths are overflow-checked. Scalar values, addresses and results
are cleared in Go before the block is cached, without an extra cgo transition.
Aggregate calls borrow an exclusive native argument vector and fixed argument/result
buffers from their plan, with arguments/results including padding zeroed before
reuse. Each plan has separate scalar and aggregate caches, each limited to four
idle contexts and 1 MiB; extra or oversized contexts are freed on return.
The aggregate budget includes retained temporary pointee storage. Each exclusive
record context may retain up to 32 cleared pointee buffers and 256 KiB inside
that budget. It borrows the smallest sufficient capacity, and no other active
context can borrow the same bytes. The entire capacity is cleared in Go before
return, including padding and pointer words. Returned pointers into unused
capacity remain subject to the same temporary lifetime checks.
An immutable aggregate argument address table restores a separate libffi vector before
every call: some backends replace argument pointers with temporary struct copies.
Retirement waits for active calls and frees every retained context. No pool
lock is held during native execution or callbacks. Callbacks use the same
prepared type storage. No fixed 32-argument limit remains; native ABI limits
and available memory still bound call size.

Equal aggregate descriptors reuse a plan-owned layout. Declared temporary
pointee layouts are constructed lazily, shared after preparation, and freed
with the plan. Temporary pointee views are valid only for their invocation;
after copy-back, cleared native bytes may be retained while unannotated type
layouts are freed. Alias maps, descriptors and
errors are cleared before a context returns to its plan. Reuse never shares
storage between overlapping or nested calls. Sharing physical ABI resources
between logically distinct signatures and cross-session sharing remain future work.
These measurements do not establish universal
latency guarantees or remove pointer lifetime and synchronization requirements.
