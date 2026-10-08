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
or replace `go` with `llgo`. The benchmarks use the same compiled C fixture and
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
recorded on 2026-10-08 with Go 1.27.0, llgo `b86d349178d6` and libffi 3.8.0.
They describe this dynamic libffi path, including logical value handling and
owner guards; they do not benchmark llgo's direct typed C adapters.

| Shape | Mode | Go ns/op | llgo ns/op | Go B/op | Go allocs/op |
| --- | --- | ---: | ---: | ---: | ---: |
| Scalar2 | Bound | 90.1 | 323.5 | 24 | 2 |
| Scalar2 | Prepared | 80.0 | 228.2 | 24 | 2 |
| Scalar2 | OneShot | 515.2 | 973.2 | 592 | 11 |
| Scalar64 | Bound | 957.7 | 1626.0 | 520 | 2 |
| Scalar64 | Prepared | 970.4 | 1508.0 | 520 | 2 |
| Scalar64 | OneShot | 5765.0 | 9150.0 | 6160 | 78 |
| Struct | Bound | 593.4 | 1505.0 | 168 | 6 |
| Struct | Prepared | 571.2 | 1372.0 | 168 | 6 |
| Struct | OneShot | 1514.0 | 2997.0 | 1320 | 26 |
| Pointer | Bound | 1617.0 | 3343.0 | 1720 | 22 |
| Pointer | Prepared | 1599.0 | 3066.0 | 1720 | 22 |
| Pointer | OneShot | 2233.0 | 4233.0 | 2784 | 37 |

The measured llgo runtime reports `0 allocs/op` for every case despite nonzero
`B/op`; that counter is not evidence of allocation-free execution. Reproduce
measurements on the application host before drawing performance conclusions.

## Storage and cache scope

CIF argument types and aggregate layouts are owned by each prepared plan.
Scalar argument bits use a Go slice sized to the call. Native scalar packing
uses a 32-slot stack buffer for small calls and overflow-checked heap storage
for larger calls. Struct calls allocate a native argument vector for their
actual count, plus independent marshaling buffers. Callbacks use the same
prepared type storage. No fixed 32-argument limit remains; native ABI limits
and available memory still bound call size.

Cross-binding CIF/layout caching, reuse of typed pointee layouts, and buffer
pooling remain future work. These measurements do not establish universal
latency guarantees or remove pointer lifetime and synchronization requirements.
