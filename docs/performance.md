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
or replace `go` with `llgo`. Use `-bench '^BenchmarkNative(Calls|BindingCache)$'` to include warm session binding. The benchmarks use the same compiled C fixture and
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

## Storage and cache scope

CIF argument types and aggregate layouts are owned by each prepared plan. Bindings with identical signature snapshots share a plan within their session; each symbol retains its own address. The signature cache uses a linear exact comparison, so binding cost also depends on the number and size of unique signatures. It does not merge ABI-equivalent but logically distinct signatures or share resources across sessions.
Scalar argument bits use a Go slice sized to the call. Native scalar packing
uses a 32-slot stack buffer for small calls and overflow-checked heap storage
for larger calls. Struct calls allocate a native argument vector for their
actual count, plus independent marshaling buffers. Callbacks use the same
prepared type storage. No fixed 32-argument limit remains; native ABI limits
and available memory still bound call size.

Equal aggregate descriptors reuse a plan-owned layout. Declared temporary
pointee layouts are constructed lazily, shared after preparation, and freed
with the plan. Unannotated temporary shapes remain invocation-owned, and value
buffers are always independent. ABI-equivalent signature normalization, indexed
cache lookup, buffer pooling and cross-session sharing remain future work. These measurements do not establish universal
latency guarantees or remove pointer lifetime and synchronization requirements.
