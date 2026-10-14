typedef unsigned UIntLow __attribute__((aligned(1)));
typedef UIntLow LowChain;
typedef unsigned UIntHigh __attribute__((aligned(16)));
typedef UIntHigh UIntDown __attribute__((aligned(2)));
typedef unsigned UIntHigher __attribute__((aligned(32)));
typedef unsigned UIntMaximum __attribute__((aligned));
typedef int IntLow __attribute__((aligned(1)));
typedef _Bool BoolLow __attribute__((aligned(1)));
enum Choice { choice_min=-1, choice_max=1 };
typedef enum Choice ChoiceHigh __attribute__((aligned(16)));
typedef struct { signed char low_tag; LowChain low_bits:3; signed char low_tail; } LowTiny;
typedef struct { signed char high_tag; UIntHigh high_bits:3; signed char high_tail; } HighTiny;
typedef struct { signed char low_wide_tag; UIntLow low_wide_bits:31; signed char low_wide_tail; } LowWide;
typedef struct { signed char high_wide_tag; UIntHigh high_wide_bits:31; signed char high_wide_tail; } HighWide;
typedef struct { signed char down_tag; UIntDown down_bits:3; IntLow down_signed:5; signed char down_tail; } Down;
typedef struct { UIntHigh shared_first:3; UIntLow shared_second:29; UIntHigher shared_third:31; signed char shared_tail; } Shared;
typedef struct { signed char float_tag; UIntHigh float_bits:3; double float_real; } FloatRecord;
typedef struct __attribute__((packed)) { signed char packed_tag; UIntHigh packed_bits:31; short packed_tail; } Packed;
#pragma pack(push,2)
typedef struct { signed char pragma_tag; UIntHigh pragma_bits:31; signed char pragma_tail; } Pragma;
#pragma pack(pop)
typedef struct { signed char max_tag; UIntMaximum max_bits:3; signed char max_tail; } Maximum;
typedef struct { BoolLow choice_active:1; ChoiceHigh choice_value:2; signed char choice_tail; } ChoiceRecord;
typedef struct { signed char envelope_tag; HighTiny envelope_items[2]; LowTiny envelope_low; } Envelope;
LowTiny echo_low(LowTiny); HighTiny echo_high(HighTiny); LowWide echo_low_wide(LowWide); HighWide echo_high_wide(HighWide);
Down echo_down(Down); Shared echo_shared(Shared); FloatRecord echo_float(FloatRecord); Packed echo_packed(Packed);
Pragma echo_pragma(Pragma); Maximum echo_maximum(Maximum); ChoiceRecord echo_choice(ChoiceRecord); Envelope echo_envelope(Envelope);
int float_variadic(FloatRecord,...); int high_variadic(HighWide,...);
typedef FloatRecord (*FloatEntry)(FloatRecord);
FloatEntry float_factory(void); FloatRecord forward_float(FloatEntry,FloatRecord);
HighWide *high_pointer(HighWide); HighWide *mutate_high(HighWide *); HighWide read_high(HighWide *);
HighTiny shifted_high(int,HighTiny);
typedef struct { signed char member_tag; UIntHigh member_bits:3 __attribute__((aligned(8))); signed char member_tail; } Member;
typedef struct { signed char member_packed_tag; UIntHigh member_packed_bits:31 __attribute__((packed)); short member_packed_tail; } MemberPacked;
Member echo_member(Member); MemberPacked echo_member_packed(MemberPacked);
typedef struct { signed char boundary_tag; UIntHigh boundary_bits:3; unsigned :0 __attribute__((aligned(8))); signed char boundary_tail; } Boundary;
typedef struct { UIntLow padding_first:3; unsigned :5; UIntHigh padding_next:31; signed char padding_tail; } SharedPadding;
Boundary echo_boundary(Boundary); SharedPadding echo_padding(SharedPadding);
#pragma pack(push,1)
typedef struct { signed char packed_parent_tag; Packed packed_parent_child; signed char packed_parent_tail; } PackedParent;
#pragma pack(pop)
PackedParent echo_packed_parent(PackedParent);
