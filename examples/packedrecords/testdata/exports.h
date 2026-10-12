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
