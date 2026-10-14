typedef struct { int child_value; } Child;
typedef Child LowChild __attribute__((aligned(1)));
#pragma pack(push,2)
typedef struct { signed char parent_tag; LowChild parent_child; unsigned parent_flags:3; unsigned :0; short parent_tail; } Parent;
typedef struct { float float_first; unsigned :0; float float_second; } FloatPair;
typedef struct { float gap_first; unsigned long long :0; float gap_second; } FloatGap;
typedef struct { signed char bits_tag; unsigned bits_flags:3; unsigned :0; short bits_tail; } Bits;
typedef struct { signed char trailing_tag; unsigned :0; } Trailing;
typedef struct { unsigned long long :0; unsigned :0; signed char leading_tag; } Leading;
typedef struct { signed char consecutive_tag; unsigned :0; unsigned :0; short consecutive_tail; } Consecutive;
typedef struct { long long nonpower_head; short nonpower_mid; unsigned :0; short nonpower_tail; } NonPower;
typedef struct { signed char large_tag; LowChild large_child; unsigned large_flags:3; unsigned :0; double large_values[3]; } Large;
typedef struct { signed char mixed_tag; unsigned mixed_flags:3; unsigned :4; unsigned long long :0; short mixed_tail; } Mixed;
#pragma pack(push,1)
typedef struct __attribute__((packed)) { signed char packed_tag; unsigned packed_flags:3; unsigned long long :0; float packed_tail; } Packed;
#pragma pack(pop)
#pragma pack(pop)
typedef struct { FloatGap envelope_gap; NonPower envelope_pair; Trailing envelope_items[2]; int envelope_check; } Envelope;
Parent echo_parent(Parent); FloatPair echo_float(FloatPair); FloatGap echo_gap(FloatGap); Bits echo_bits(Bits);
Trailing echo_trailing(Trailing); Leading echo_leading(Leading); Consecutive echo_consecutive(Consecutive); NonPower echo_nonpower(NonPower);
Large echo_large(Large); Mixed echo_mixed(Mixed); Packed echo_packed(Packed); Envelope echo_envelope(Envelope);
FloatPair shifted_float(int,FloatPair);
double gap_overflow(double,double,double,double,double,double,double,double,FloatGap,double);
double gap_variadic(double,double,double,double,double,double,double,double,FloatGap,...);
typedef FloatPair (*FloatEntry)(FloatPair);
FloatEntry float_factory(void); FloatPair forward_float(FloatEntry,FloatPair);
FloatGap *gap_pointer(FloatGap); FloatGap read_gap(FloatGap *);
