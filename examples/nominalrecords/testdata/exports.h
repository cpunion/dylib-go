typedef struct { float x; int y; } Plain;
typedef struct __attribute__((packed)) { float x; int y; } Packed;
typedef struct __attribute__((aligned(16))) { float x; int y; } Aligned;
typedef Plain PlainAlias;
typedef struct { Plain a[2]; Packed b[2]; Aligned c[2]; } Envelope;
Plain echo_plain(Plain); Packed echo_packed(Packed); Aligned echo_aligned(Aligned);
PlainAlias echo_alias(PlainAlias); Envelope echo_envelope(Envelope);
typedef Packed (*PackedFn)(Packed);
PackedFn packed_factory(void); Packed apply_packed(PackedFn, Packed);
Plain *plain_pointer(Plain); Packed *packed_pointer(Packed);
Plain read_plain(Plain *); Packed read_packed(Packed *);
Packed packed_variable(Packed,...);
#if defined(__APPLE__) && defined(__aarch64__)
typedef struct { float x; unsigned long long :33; signed char y; } Wide;
typedef struct { float x; unsigned long long :3; signed char y; } Small;
Wide echo_wide(Wide); Small echo_small(Small);
#endif
