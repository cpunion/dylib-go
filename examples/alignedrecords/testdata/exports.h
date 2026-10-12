typedef struct __attribute__((aligned(32))) {
    signed char tag;
    double value;
    int tail;
    void *context;
} Aligned;
typedef struct {
    signed char tag;
    _Alignas(16) double values[2];
    int count;
} FieldAligned;
typedef struct __attribute__((packed,aligned(32))) {
    signed char tag;
    double value;
    short tail;
} PackedAligned;
typedef struct __attribute__((packed,aligned(16))) {
    unsigned tag:3;
    signed delta:5;
    unsigned :0;
    double value;
    unsigned enabled:1;
} AlignedBits;
typedef struct {
    Aligned items[2];
    FieldAligned fields[2];
    PackedAligned packed[2];
    AlignedBits bits[2];
    int count;
} Envelope;
typedef struct __attribute__((aligned(32))) { double first; double second; } AlignedFloats;
typedef struct {
    unsigned low:5;
    int value __attribute__((aligned(16)));
    signed high:7 __attribute__((aligned(8)));
    unsigned :0;
    signed char tag;
} FieldAlignedBits;
typedef struct __attribute__((packed,aligned(32))) {
    signed char tag;
    double values[2] __attribute__((aligned(16)));
    short tail;
} PackedFields;

Aligned echo_aligned(Aligned);
FieldAligned echo_fields(FieldAligned);
PackedAligned echo_packed(PackedAligned);
AlignedBits echo_bits(AlignedBits);
Envelope echo_envelope(Envelope);
typedef Aligned (*AlignedFn)(Aligned);
AlignedFn aligned_factory(void);
Aligned apply_aligned(AlignedFn, Aligned);
Aligned *aligned_pointer(void);
Aligned *mutate_aligned(Aligned *);
double sum_aligned(Aligned *);
Aligned aligned_variable(Aligned, ...);
AlignedFloats echo_floats(AlignedFloats);
FieldAlignedBits echo_field_bits(FieldAlignedBits);
PackedFields echo_packed_fields(PackedFields);
unsigned long long aligned_address(Aligned *);
