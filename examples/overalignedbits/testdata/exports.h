#pragma pack(push,2)
typedef struct { unsigned tiny_low:3 __attribute__((aligned(8))); signed tiny_high:5 __attribute__((aligned(32))); } Tiny;
typedef struct {
    signed char tag;
    unsigned low:3 __attribute__((aligned(16)));
    signed high:5 __attribute__((aligned(32)));
    unsigned long long :0 __attribute__((aligned(32)));
    _Bool truth:1 __attribute__((aligned(16)));
    unsigned long long wide:40 __attribute__((packed,aligned(32)));
    double real __attribute__((aligned(16)));
    int count;
    void *context;
} Bits;
typedef struct {
    unsigned long long :0 __attribute__((aligned(32)));
    unsigned :3 __attribute__((aligned(16)));
    signed pad_high:5;
    unsigned long long :0 __attribute__((aligned(32)));
    unsigned :0 __attribute__((aligned(64)));
    float samples[2] __attribute__((aligned(16)));
    void *pad_context;
} Padding;
#pragma pack(pop)
typedef struct __attribute__((aligned(32))) { unsigned inner_low:3; float inner_samples[2]; } Inner;
#pragma pack(push,2)
typedef struct { signed char envelope_tag; Bits items[2][2]; Padding pads[2]; Inner inners[1]; int envelope_count; } Envelope;
typedef struct { unsigned long long :0 __attribute__((aligned(32))); float left; float right; unsigned :0 __attribute__((aligned(16))); } Floating;
typedef struct __attribute__((packed,aligned(32))) { signed char packed_tag; unsigned :3 __attribute__((aligned(16))); signed packed_high:5; unsigned long long :0 __attribute__((aligned(32))); double packed_real __attribute__((packed,aligned(16))); } Packed;
typedef struct { unsigned :3 __attribute__((aligned(16))); float gap_left; float gap_right; } PaddingFloat;
#pragma pack(pop)
Tiny echo_tiny(Tiny);
Bits echo_bits(Bits);
Padding echo_padding(Padding);
Inner echo_inner(Inner);
Envelope echo_envelope(Envelope);
Floating echo_floating(Floating);
Packed echo_packed(Packed);
PaddingFloat echo_padding_float(PaddingFloat);
typedef Bits (*BitsFn)(Bits);
BitsFn bits_factory(void);
Bits apply_bits(BitsFn,Bits);
Bits *bits_pointer(void);
Bits *mutate_bits(Bits *);
double sum_bits(Bits *);
Bits bits_variable(Bits,...);
Tiny tiny_variable(Tiny,...);
Floating floating_variable(Floating,...);
#pragma pack(push,2)
typedef struct { signed char slot_tag; BitsFn entry __attribute__((aligned(16))); unsigned slot_flags:3; } Slot;
#pragma pack(pop)
Bits apply_slot(Slot,Bits);
#pragma pack(push,2)
typedef struct { unsigned tail_low:3 __attribute__((aligned(8))); signed tail_high:5 __attribute__((aligned(32))); unsigned long long :0 __attribute__((aligned(16))); } TinyTail;
typedef struct { float triple_left,triple_middle,triple_right; unsigned long long :0 __attribute__((aligned(16))); } FloatingTriple;
typedef struct { float single_left; unsigned :0 __attribute__((aligned(8))); } FloatingSingle;
#pragma pack(pop)
TinyTail echo_tiny_tail(TinyTail);
TinyTail tiny_tail_variable(TinyTail,...);
FloatingTriple echo_triple(FloatingTriple);
FloatingTriple triple_variable(FloatingTriple,...);
FloatingSingle echo_single(FloatingSingle);
FloatingSingle single_variable(FloatingSingle,...);
