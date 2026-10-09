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

These figures cover the small fixtures above. Returned aggregates still require
logical Go values, temporary pointees still use per-call native allocation, and
large contexts exceeding the idle budget are freed rather than retained. The
scalar-only path is unchanged. Native allocations are excluded from the Go
allocation counters; llgo's reported allocation count has the limitation
described above.

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
Scalar argument bits use a Go slice sized to the call. Native scalar packing
uses a 32-slot stack buffer for small calls and overflow-checked heap storage
for larger calls. Aggregate calls borrow an exclusive native argument vector
and fixed argument/result buffers from their plan. Idle storage is limited to
four contexts and 1 MiB of native buffers per plan; extra contexts are freed
on return. Arguments/results, including padding, are zeroed before reuse.
An immutable argument address table restores a separate libffi vector before
every call: some backends replace argument pointers with temporary struct copies.
Retirement waits for active calls and frees every retained context. No pool
lock is held during native execution or callbacks. Callbacks use the same
prepared type storage. No fixed 32-argument limit remains; native ABI limits
and available memory still bound call size.

Equal aggregate descriptors reuse a plan-owned layout. Declared temporary
pointee layouts are constructed lazily, shared after preparation, and freed
with the plan. Temporary pointee memory and unannotated type layouts remain
invocation-owned and are freed after copy-back. Alias maps, descriptors and
errors are cleared before a context returns to its plan. Reuse never shares
storage between overlapping or nested calls. Sharing physical ABI resources
between logically distinct signatures, scalar buffer pooling, temporary pointee
buffer pooling and cross-session sharing remain future work.
These measurements do not establish universal
latency guarantees or remove pointer lifetime and synchronization requirements.
