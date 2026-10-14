#include "exports.h"
#if defined(_MSC_VER)
int _fltused=0x9875;
#endif
#define ECHO(T,N) T N(T value){return value;}
ECHO(LowTiny,echo_low) ECHO(HighTiny,echo_high) ECHO(LowWide,echo_low_wide) ECHO(HighWide,echo_high_wide)
ECHO(Down,echo_down) ECHO(Shared,echo_shared) ECHO(FloatRecord,echo_float) ECHO(Packed,echo_packed)
ECHO(Pragma,echo_pragma) ECHO(Maximum,echo_maximum) ECHO(ChoiceRecord,echo_choice) ECHO(Envelope,echo_envelope)
#define VAR(T,N,V) int N(T value,...){__builtin_va_list ap;__builtin_va_start(ap,value);int a=__builtin_va_arg(ap,int);double b=__builtin_va_arg(ap,double);__builtin_va_end(ap);return (V)+a+(int)b;}
VAR(FloatRecord,float_variadic,value.float_tag+(int)value.float_bits+(int)value.float_real)
VAR(HighWide,high_variadic,value.high_wide_tag+(int)value.high_wide_bits+value.high_wide_tail)
FloatEntry float_factory(void){return echo_float;}
FloatRecord forward_float(FloatEntry entry,FloatRecord v){return entry(v);}
static HighWide storage;
HighWide *high_pointer(HighWide v){storage=v;return &storage;}
HighWide *mutate_high(HighWide *v){if(v){v->high_wide_bits+=1;v->high_wide_tail+=2;}return v;}
HighWide read_high(HighWide *v){return *v;}
HighTiny shifted_high(int marker,HighTiny v){v.high_tag+=marker;return v;}
ECHO(Member,echo_member) ECHO(MemberPacked,echo_member_packed)
ECHO(Boundary,echo_boundary) ECHO(SharedPadding,echo_padding)
ECHO(PackedParent,echo_packed_parent)
