typedef struct {
    unsigned flags:3 __attribute__((packed));
    signed delta:5;
    _Bool active:1;
    unsigned long long wide:40 __attribute__((packed,aligned(16)));
    double value;
    void *context;
} Bits;
typedef struct __attribute__((packed)) {
    unsigned a:3;
    signed b:5;
    _Bool on:1;
    unsigned long long big:40;
    double real;
} PackedBits;
typedef struct { Bits regular; PackedBits packed[2][2]; } Envelope;
Bits echo_bits(Bits);
PackedBits echo_packed_bits(PackedBits);
Envelope echo_envelope(Envelope);
typedef Bits (*BitsFn)(Bits);
BitsFn bits_factory(void);
Bits apply_bits(BitsFn, Bits);
Bits *bits_pointer(void);
Bits *mutate_bits(Bits *);
double sum_bits(Bits *);
Bits bits_variable(Bits, ...);
typedef struct { signed long long value:40; unsigned long long all:64; } SignedWide;
SignedWide echo_signed_wide(SignedWide);
typedef struct {
    unsigned :3 __attribute__((packed,aligned));
    unsigned low:5 __attribute__((packed));
    unsigned :0 __attribute__((aligned(16)));
    signed high:7 __attribute__((packed,aligned(2)));
    unsigned :4;
    unsigned long long :0;
    unsigned :0;
    _Bool enabled:1;
    unsigned :0;
    unsigned value;
    unsigned :5;
} GapBits;
typedef struct { unsigned :7 __attribute__((packed,aligned(16))); unsigned char octet; } PrefixBits;
typedef struct __attribute__((packed)) {
    unsigned :3 __attribute__((packed,aligned(16)));
    unsigned low:5;
    unsigned :0 __attribute__((aligned(16)));
    signed high:7;
    unsigned :1;
    _Bool enabled:1;
    double real;
    unsigned :5;
} PackedGapBits;
typedef struct { GapBits items[2]; PrefixBits prefix; PackedGapBits packed[2]; } GapEnvelope;
GapBits echo_gaps(GapBits);
PrefixBits echo_prefix(PrefixBits);
PackedGapBits echo_packed_gaps(PackedGapBits);
GapEnvelope echo_gap_envelope(GapEnvelope);
GapBits *gaps_pointer(void);
GapBits *mutate_gaps(GapBits *);
unsigned sum_gaps(GapBits *);
typedef GapBits (*GapFn)(GapBits);
GapFn gaps_factory(void);
GapBits apply_gaps(GapFn, GapBits);
GapBits gaps_variable(GapBits, ...);
typedef struct __attribute__((packed)) { unsigned :3; double first; unsigned :0; double second; unsigned :5; } FloatingGap;
FloatingGap echo_floating_gap(FloatingGap);
typedef struct { unsigned low:3 __attribute__((packed)); signed high:5 __attribute__((packed)); } MemberTiny;
MemberTiny echo_member_tiny(MemberTiny);
typedef struct { unsigned code:3 __attribute__((packed)); unsigned :4; unsigned long long :0; unsigned :0; } MemberTrailing;
MemberTrailing echo_member_trailing(MemberTrailing);
typedef struct { signed char marker; unsigned code:3 __attribute__((packed)); unsigned :0 __attribute__((packed)); signed char tail; } PackedZero;
PackedZero echo_packed_zero(PackedZero);

typedef struct { float left; unsigned :0 __attribute__((aligned(16))); float right; } IgnoredFloatZero;
IgnoredFloatZero echo_ignored_float_zero(IgnoredFloatZero);
typedef struct { signed char aligned_marker; unsigned :3 __attribute__((aligned(16))); unsigned aligned_code:5; unsigned :5 __attribute__((aligned(32))); signed char aligned_tail; } AlignedUnit;
AlignedUnit echo_aligned_unit(AlignedUnit);
typedef struct __attribute__((packed)) { signed char packed_marker; unsigned :3 __attribute__((aligned(16))); unsigned packed_code:5; unsigned :5 __attribute__((aligned(32))); signed char packed_tail; } PackedAlignedUnit;
PackedAlignedUnit echo_packed_aligned_unit(PackedAlignedUnit);

#pragma pack(push,2)
typedef struct {
    signed char pragma_tag;
    unsigned :3 __attribute__((aligned(2)));
    unsigned pragma_low:5 __attribute__((packed));
    unsigned :0;
    unsigned long long :0;
    signed pragma_high:7 __attribute__((packed));
    unsigned :0 __attribute__((packed));
    _Bool pragma_active:1;
    unsigned long long pragma_wide:40;
    double pragma_samples[2];
    void *pragma_context;
    unsigned :5;
} PragmaBits;
typedef struct __attribute__((packed)) {
    signed char packed_tag;
    unsigned :3 __attribute__((aligned(2)));
    unsigned packed_low:5;
    unsigned long long :0;
    signed packed_high:7;
    unsigned :0;
    double packed_real;
} PragmaPacked;
typedef struct { float float_left; unsigned long long :0; float float_right; } PragmaFloat;
#pragma pack(push,1)
typedef struct { unsigned tiny_low:3; signed tiny_high:5; unsigned :4; unsigned long long :0; unsigned :0; } PragmaTiny;
#pragma pack(pop)
#pragma pack(pop)
#pragma pack(push,4)
typedef struct { signed char envelope_tag; PragmaBits bits[2][2]; PragmaPacked packed[2]; PragmaTiny tiny; PragmaFloat floating; } PragmaBitsEnvelope;
#pragma pack(pop)
PragmaBits echo_pragma_bits(PragmaBits);
PragmaTiny echo_pragma_tiny(PragmaTiny);
PragmaPacked echo_pragma_packed(PragmaPacked);
PragmaFloat echo_pragma_float(PragmaFloat);
PragmaBitsEnvelope echo_pragma_bits_envelope(PragmaBitsEnvelope);
typedef PragmaBits (*PragmaBitsFn)(PragmaBits);
PragmaBitsFn pragma_bits_factory(void);
PragmaBits apply_pragma_bits(PragmaBitsFn, PragmaBits);
PragmaBits *pragma_bits_pointer(void);
PragmaBits *mutate_pragma_bits(PragmaBits *);
double sum_pragma_bits(PragmaBits *);
PragmaBits pragma_bits_variable(PragmaBits, ...);
#pragma pack(push,2)
typedef struct { signed char slot_tag; PragmaBitsFn entry; } PragmaBitsSlot;
#pragma pack(pop)
PragmaBits apply_pragma_bits_slot(PragmaBitsSlot, PragmaBits);
