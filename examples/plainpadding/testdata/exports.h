#ifndef DYLIB_PLAIN_PADDING_H
#define DYLIB_PLAIN_PADDING_H

typedef struct { float wide_real; unsigned long long :33; signed char wide_tag; } PlainWide;
typedef struct { unsigned long long :3; float lead_real; } PlainLead;
typedef struct { float tail_real; unsigned long long :40; } PlainTail;
typedef struct { float pair_left; unsigned :3; float pair_right; } PlainPair;
typedef struct { signed char mixed_tag; unsigned long long :64; double mixed_real; } PlainMixed;
typedef struct { void *context; unsigned long long :3; float pointer_real; } PlainPointer;
typedef struct { unsigned long long :3; signed char multiple_tag; unsigned :3; unsigned long long :33; float multiple_real; unsigned long long :3; } PlainMultiple;
typedef struct { unsigned long long :1; float array_values[2][2]; unsigned long long :33; } PlainArray;
typedef struct { signed char envelope_tag; PlainWide items[2][2]; PlainLead leads[2]; double envelope_real; } PlainEnvelope;

PlainWide echo_wide(PlainWide);
PlainLead echo_lead(PlainLead);
PlainTail echo_tail(PlainTail);
PlainPair echo_pair(PlainPair);
PlainMixed echo_mixed(PlainMixed);
PlainPointer echo_pointer(PlainPointer);
PlainMultiple echo_multiple(PlainMultiple);
PlainArray echo_array(PlainArray);
PlainEnvelope echo_envelope(PlainEnvelope);
PlainWide plain_variable(PlainWide,...);
PlainWide plain_overflow(long long,long long,long long,long long,long long,long long,long long,long long,double,double,double,double,double,double,double,double,PlainWide);
PlainWide (*plain_factory(void))(PlainWide);
PlainWide plain_forward(PlainWide (*)(PlainWide),PlainWide);
PlainWide *plain_pointer(void);
PlainWide plain_read(PlainWide *);
void plain_write(PlainWide *,PlainWide);
PlainWide plain_record_variable(int,...);
#endif
