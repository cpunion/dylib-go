typedef struct __attribute__((packed)) {
    signed char tag;
    double value;
    short tail;
    _Bool truth;
    void *context;
} Packed;
typedef struct { Packed items[2][2]; int values[2]; } Envelope;
Packed echo_packed(Packed);
Envelope echo_envelope(Envelope);
typedef Packed (*PackedFn)(Packed);
PackedFn packed_factory(void);
Packed apply_packed(PackedFn, Packed);
Packed *packed_pointer(void);
Packed *mutate_packed(Packed *);
double sum_packed(Packed *);
Packed packed_variable(Packed, ...);
typedef struct {
    signed char tag;
    double values[2] __attribute__((packed));
    short tail;
    void *context __attribute__((packed));
} Member;
typedef struct { Member items[2][2]; int count; } MemberEnvelope;
typedef struct {
    signed char marker;
    double samples[2] __attribute__((packed,aligned(4)));
    int counter;
    _Bool truth __attribute__((packed));
} MemberAligned;
Member echo_member(Member);
MemberEnvelope echo_members(MemberEnvelope);
MemberAligned echo_member_aligned(MemberAligned);
typedef Member (*MemberFn)(Member);
MemberFn member_factory(void);
Member apply_member(MemberFn,Member);
Member *member_pointer(void);
Member *mutate_member(Member *);
double sum_member(Member *);
Member member_variable(Member,...);

#pragma pack(push,2)
typedef struct {
    signed char pragma_tag;
    double pragma_samples[2];
    int pragma_count;
    _Bool pragma_truth;
    void *pragma_context;
} Pragma;
#pragma pack(push,4)
typedef struct { signed char envelope_tag; Pragma pragma_items[2][2]; int checksum; } PragmaEnvelope;
#pragma pack(pop)
typedef struct __attribute__((aligned(32))) { signed char aligned_tag; int aligned_count; double aligned_value; } PragmaAligned;
#pragma pack(pop)
typedef struct { PragmaEnvelope payload; double natural_tail; } PragmaOuter;
Pragma echo_pragma(Pragma);
PragmaEnvelope echo_pragma_envelope(PragmaEnvelope);
PragmaAligned echo_pragma_aligned(PragmaAligned);
PragmaOuter echo_pragma_outer(PragmaOuter);
typedef Pragma (*PragmaFn)(Pragma);
PragmaFn pragma_factory(void);
Pragma apply_pragma(PragmaFn,Pragma);
Pragma *pragma_pointer(void);
Pragma *mutate_pragma(Pragma *);
double sum_pragma(Pragma *);
Pragma pragma_variable(Pragma,...);

#pragma pack(push,2)
typedef struct { signed char slot_tag; PragmaFn entry; } PragmaSlot;
#pragma pack(pop)
Pragma apply_pragma_slot(PragmaSlot,Pragma);

#pragma pack(push,2)
typedef struct {
    signed char over_tag;
    int over_count __attribute__((aligned(16)));
    double over_samples[2];
    _Bool over_truth;
    void *over_context;
} PragmaOver;
typedef struct __attribute__((packed)) { signed char packed_tag; int packed_count __attribute__((aligned(16))); double packed_real; } PragmaOverPacked;
typedef struct { signed char max_tag; int max_count __attribute__((aligned)); double max_real; } PragmaOverMax;
typedef struct { signed char type_tag; _Alignas(PragmaAligned) int type_count; double type_real; } PragmaOverType;
typedef struct { signed char array_tag; double array_values[2] __attribute__((aligned(32))); int array_count; } PragmaOverArray;
#pragma pack(push,4)
typedef struct {
    signed char nested_tag;
    PragmaOver nested_items[2][2];
    PragmaOverPacked nested_packed[2];
    PragmaOverMax nested_max[2];
    PragmaOverType nested_type[2];
    PragmaOverArray nested_array[2];
    PragmaAligned nested_aligned[2];
    int nested_count;
} PragmaOverEnvelope;
#pragma pack(pop)
typedef struct __attribute__((aligned(32))) { float over_left; float over_right; } PragmaOverFloat;
#pragma pack(pop)
typedef struct { PragmaOverEnvelope over_payload; double over_tail; } PragmaOverOuter;
PragmaOver echo_over(PragmaOver);
PragmaOverPacked echo_over_packed(PragmaOverPacked);
PragmaOverMax echo_over_max(PragmaOverMax);
PragmaOverType echo_over_type(PragmaOverType);
PragmaOverArray echo_over_array(PragmaOverArray);
PragmaOverEnvelope echo_over_envelope(PragmaOverEnvelope);
PragmaOverFloat echo_over_float(PragmaOverFloat);
PragmaOverOuter echo_over_outer(PragmaOverOuter);
typedef PragmaOver (*PragmaOverFn)(PragmaOver);
PragmaOverFn over_factory(void);
PragmaOver apply_over(PragmaOverFn,PragmaOver);
PragmaOver *over_pointer(void);
PragmaOver *mutate_over(PragmaOver *);
double sum_over(PragmaOver *);
PragmaOver over_variable(PragmaOver,...);
#pragma pack(push,2)
typedef struct { signed char over_slot_tag; PragmaOverFn over_entry __attribute__((aligned(16))); } PragmaOverSlot;
#pragma pack(pop)
PragmaOver apply_over_slot(PragmaOverSlot,PragmaOver);
#pragma pack(push,4)
typedef struct { PragmaOverFloat over_floats[1]; float over_float_tail; } PragmaOverFloatNested;
#pragma pack(pop)
PragmaOverFloatNested echo_over_float_nested(PragmaOverFloatNested);
