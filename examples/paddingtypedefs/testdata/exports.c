#include "exports.h"
#if defined(_MSC_VER)
int _fltused=0x9875;
#endif
#define ECHO(T,N) T N(T v){return v;}
ECHO(LowTiny,echo_low) ECHO(HighTiny,echo_high) ECHO(LowWide,echo_low_wide) ECHO(HighWide,echo_high_wide)
ECHO(Zero,echo_zero) ECHO(Active,echo_active) ECHO(ZeroLow,echo_zero_low) ECHO(Shared,echo_shared)
ECHO(FloatRecord,echo_float) ECHO(Packed,echo_packed) ECHO(Pragma,echo_pragma) ECHO(Maximum,echo_maximum)
ECHO(ChoiceRecord,echo_choice) ECHO(Envelope,echo_envelope) ECHO(MemberPacked,echo_member)
ECHO(SmallZero,echo_small) ECHO(TailFloat,echo_tail) ECHO(Down,echo_down) ECHO(Consecutive,echo_consecutive)
ECHO(PackedParent,echo_parent) ECHO(MaximumZero,echo_maximum_zero) ECHO(Leading,echo_leading)
#define VAR(T,N,V) int N(T v,...){__builtin_va_list ap;__builtin_va_start(ap,v);int a=__builtin_va_arg(ap,int);double b=__builtin_va_arg(ap,double);__builtin_va_end(ap);return (V)+a+(int)b;}
VAR(FloatRecord,float_variadic,v.float_tag+(int)v.float_real)
VAR(SmallZero,small_variadic,(int)v.small_value+v.small_tail)
FloatEntry float_factory(void){return echo_float;}
FloatRecord forward_float(FloatEntry entry,FloatRecord v){return entry(v);}
static HighWide storage;
HighWide *high_pointer(HighWide v){storage=v;return &storage;}
HighWide *mutate_high(HighWide *v){if(v){v->high_wide_tail+=2;}return v;}
HighWide read_high(HighWide *v){return *v;}
HighTiny shifted_high(int marker,HighTiny v){v.high_tag+=marker;return v;}
SmallZero shifted_small(int marker,SmallZero v){v.small_tail+=marker;return v;}
double small_overflow(double a,double b,double c,double d,double e,double f,double g,double h,SmallZero v,double tail){return a+b+c+d+e+f+g+h+v.small_value+v.small_tail+tail;}

ECHO(MemberAligned,echo_aligned)
