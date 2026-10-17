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
typedef struct { float bare_left; } FloatingBare;
#pragma pack(push,2)
typedef struct { double double_left; unsigned :0 __attribute__((aligned(8))); } FloatingDouble;
typedef struct { float array_values[1]; unsigned :0 __attribute__((aligned(8))); } FloatingArray;
typedef struct { FloatingBare nested; } FloatingNested;
#pragma pack(pop)
FloatingBare echo_bare(FloatingBare);
FloatingDouble echo_double(FloatingDouble);
FloatingArray echo_array(FloatingArray);
FloatingNested echo_nested(FloatingNested);
#pragma pack(push,2)
typedef struct { float interior_left; unsigned :0 __attribute__((aligned(8))); float interior_right; } GapFloat;
typedef struct { float tail_left; unsigned :0 __attribute__((aligned(8))); float tail_right; unsigned :0 __attribute__((aligned(16))); } GapTail;
typedef struct { unsigned gap_low:3; signed gap_high:5; unsigned :0 __attribute__((aligned(8))); float bits_right; } GapBits;
typedef struct { signed char gap_tag; unsigned :0 __attribute__((aligned(4))); float byte_right; } GapByte;
typedef struct { FloatingBare gap_nested; unsigned :0 __attribute__((aligned(8))); float nested_right; } GapNested;
typedef struct { float gap_array[1]; unsigned :0 __attribute__((aligned(8))); float array_right; } GapArray;
typedef struct { signed char first_tag; unsigned :0 __attribute__((aligned(4))); short next_code; unsigned :0 __attribute__((aligned(8))); float multi_right; } GapMultiple;
#pragma pack(pop)
GapFloat echo_gap_float(GapFloat);
GapFloat gap_float_variable(GapFloat,...);
GapTail echo_gap_tail(GapTail);
GapTail gap_tail_variable(GapTail,...);
GapBits echo_gap_bits(GapBits);
GapBits gap_bits_variable(GapBits,...);
GapByte echo_gap_byte(GapByte);
GapByte gap_byte_variable(GapByte,...);
GapNested echo_gap_nested(GapNested);
GapNested gap_nested_variable(GapNested,...);
GapArray echo_gap_array(GapArray);
GapArray gap_array_variable(GapArray,...);
GapMultiple echo_gap_multiple(GapMultiple);
GapMultiple gap_multiple_variable(GapMultiple,...);

#pragma pack(push,2)
typedef struct { unsigned :1; signed char byte_tag; unsigned :0 __attribute__((aligned(4))); float byte_real; } BytePrefix;
typedef struct { unsigned bit_flags:3; unsigned :5; unsigned :0 __attribute__((aligned(8))); double bit_real; } BitPrefix;
typedef struct { float float_first; unsigned :3; signed char float_tag; unsigned :0 __attribute__((aligned(8))); float float_second; } FloatPrefix;
typedef struct { signed char multi_tag; unsigned :3; unsigned :0 __attribute__((aligned(4))); short multi_code; unsigned :0 __attribute__((aligned(8))); float multi_real; } MultiPrefix;
typedef unsigned long long Boundary __attribute__((aligned(8)));
typedef struct { unsigned :1; signed char typedef_tag; Boundary :0; double typedef_real; } TypedefPrefix;
#pragma pack(pop)
typedef struct { BytePrefix prefix_items[2]; BitPrefix prefix_bits; } PrefixEnvelope;
BytePrefix echo_byte_prefix(BytePrefix); BitPrefix echo_bit_prefix(BitPrefix);
FloatPrefix echo_float_prefix(FloatPrefix); MultiPrefix echo_multi_prefix(MultiPrefix);
TypedefPrefix echo_typedef_prefix(TypedefPrefix);
PrefixEnvelope echo_prefix_envelope(PrefixEnvelope);

BitPrefix prefix_variable(BitPrefix,...);
double prefix_overflow(long long,long long,long long,long long,long long,long long,double,double,double,double,double,double,double,double,BitPrefix,long long);
typedef BitPrefix (*PrefixEntry)(BitPrefix);
PrefixEntry prefix_factory(void); BitPrefix forward_prefix(PrefixEntry,BitPrefix);
BitPrefix *prefix_pointer(BitPrefix); BitPrefix read_prefix(BitPrefix *);

#pragma pack(push,2)
typedef struct __attribute__((packed)) { unsigned :1; signed char root_byte_tag; unsigned :0 __attribute__((aligned(4))); float root_byte_real; } PackedRootByte;
#pragma pack(pop)
#pragma pack(push,4)
typedef struct __attribute__((packed)) { unsigned long long :31; signed char root_leading_tag; float root_leading_real; } PackedRootLeading;
typedef struct __attribute__((packed)) { unsigned root_named_flags:3; unsigned long long :40; signed char root_named_tag; float root_named_real; } PackedRootNamed;
typedef struct __attribute__((packed)) { float root_floating_head; unsigned :3; signed char root_floating_tag; double root_floating_real; } PackedRootFloating;
typedef struct __attribute__((packed,aligned(16))) { unsigned :3; signed char root_aligned_tag; double root_aligned_real; } PackedRootAligned;
#pragma pack(pop)
typedef struct { PackedRootByte root_items[2]; PackedRootNamed root_bits; PackedRootFloating root_floats; } PackedRootEnvelope;
PackedRootByte echo_root_byte(PackedRootByte); PackedRootLeading echo_root_leading(PackedRootLeading);
PackedRootNamed echo_root_named(PackedRootNamed); PackedRootFloating echo_root_floating(PackedRootFloating);
PackedRootAligned echo_root_aligned(PackedRootAligned); PackedRootEnvelope echo_root_envelope(PackedRootEnvelope);
PackedRootByte root_variable(PackedRootByte,...);
double root_overflow(long long,long long,long long,long long,long long,long long,double,double,double,double,double,double,double,double,PackedRootByte,long long);
typedef PackedRootByte (*RootEntry)(PackedRootByte);
RootEntry root_factory(void); PackedRootByte forward_root(RootEntry,PackedRootByte);
PackedRootEnvelope *root_pointer(PackedRootEnvelope); PackedRootEnvelope read_root(PackedRootEnvelope *);
