typedef struct {
    unsigned flags:3;
    signed delta:5;
    _Bool active:1;
    unsigned long long wide:40;
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
