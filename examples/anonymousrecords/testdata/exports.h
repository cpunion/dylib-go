typedef struct { int high_value; } High __attribute__((aligned(16)));
typedef High Inherited;
typedef High Low __attribute__((aligned(1)));
typedef struct { int low_value; } LowOnly __attribute__((aligned(1)));
typedef struct { float scalar; } Float __attribute__((aligned(16)));
typedef struct { long long first,second,third; } Wide __attribute__((aligned(16)));
typedef Wide WideLow __attribute__((aligned(1)));
typedef struct { double real_first,real_second; } Doubles __attribute__((aligned(1)));
typedef struct { int omitted_value; } Maximum __attribute__((aligned));
typedef struct __attribute__((packed)) { signed char packed_tag; High packed_child; int packed_tail; } Packed __attribute__((aligned(32)));
#pragma pack(push,2)
typedef struct { signed char pragma_tag; Low pragma_child; unsigned pragma_flags:3; unsigned :0 __attribute__((aligned(4))); short pragma_tail; } Pragma __attribute__((aligned(16)));
#pragma pack(pop)
typedef struct { signed char member_tag; Low member_child __attribute__((aligned(8))); signed char member_tail; } Member __attribute__((aligned(32)));
typedef unsigned BitBase __attribute__((aligned(16)));
typedef struct { signed char bits_tag; BitBase bits_flags:3; unsigned :0; signed char bits_tail; } Bits __attribute__((aligned(32)));
typedef struct { signed char nested_tag; Low nested_child; WideLow nested_wide; } Nested __attribute__((aligned(32)));
typedef struct { long long pair_left,pair_right; } Pair __attribute__((aligned(16)));
typedef struct { signed char array_tag; Pair array_values[2]; } Array __attribute__((aligned(32)));
typedef struct __attribute__((packed,aligned(8))) { signed char explicit_tag; int explicit_value; } Explicit __attribute__((aligned(32)));
High echo_high(High); Inherited echo_inherited(Inherited); Low echo_low(Low); LowOnly echo_low_only(LowOnly);
Float echo_float(Float); Wide echo_wide(Wide); WideLow echo_wide_low(WideLow); Doubles echo_doubles(Doubles);
Maximum echo_maximum(Maximum); Packed echo_packed(Packed); Pragma echo_pragma(Pragma); Member echo_member(Member);
Bits echo_bits(Bits); Nested echo_nested(Nested); Array echo_array(Array); Explicit echo_explicit(Explicit);
High shifted(int,High); double overflow(double,double,double,double,double,double,double,double,Float,double);
long long wide_overflow(int,int,int,int,int,int,int,int,WideLow,int);
double variadic(double,double,double,double,double,double,double,double,Float,...);
typedef High (*HighEntry)(High); HighEntry factory(void); High forward(HighEntry,High);
High *pointer(High); High read(High *); int alignment(High *);
