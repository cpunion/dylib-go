#include "exports.h"
#if defined(_MSC_VER)
int _fltused=0x9875;
#endif
#define ECHO(T,N) T N(T value){return value;}
ECHO(Parent,echo_parent) ECHO(FloatPair,echo_float) ECHO(FloatGap,echo_gap) ECHO(Bits,echo_bits)
ECHO(Trailing,echo_trailing) ECHO(Leading,echo_leading) ECHO(Consecutive,echo_consecutive) ECHO(NonPower,echo_nonpower)
ECHO(Large,echo_large) ECHO(Mixed,echo_mixed) ECHO(Packed,echo_packed) ECHO(Envelope,echo_envelope)
FloatPair shifted_float(int marker,FloatPair value){value.float_first+=marker;return value;}
#define PREFIX double a,double b,double c,double d,double e,double f,double g,double h
#define SUM (a+b+c+d+e+f+g+h)
double gap_overflow(PREFIX,FloatGap value,double tail){return SUM+value.gap_first+value.gap_second+tail;}
double gap_variadic(PREFIX,FloatGap value,...){__builtin_va_list args;__builtin_va_start(args,value);int marker=__builtin_va_arg(args,int);double tail=__builtin_va_arg(args,double);__builtin_va_end(args);return SUM+value.gap_first+value.gap_second+marker+tail;}
FloatEntry float_factory(void){return echo_float;}
FloatPair forward_float(FloatEntry entry,FloatPair value){return entry(value);}
FloatGap *gap_pointer(FloatGap value){static FloatGap storage;storage=value;return &storage;}
FloatGap read_gap(FloatGap *pointer){FloatGap value={0};if(pointer)value=*pointer;return value;}
