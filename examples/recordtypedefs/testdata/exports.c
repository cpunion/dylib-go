#include "exports.h"
#if defined(_MSC_VER)
int _fltused=0x9875;
#endif
#define ECHO(T,name) T name(T v) { return v; }
ECHO(Plain,echo_plain) ECHO(High,echo_high) ECHO(Low,echo_low) ECHO(Inherited,echo_inherited)
ECHO(WideHigh,echo_wide) ECHO(WideLow,echo_wide_low) ECHO(FloatHigh,echo_float) ECHO(FloatLow,echo_float_low)
ECHO(LowParent,echo_low_parent) ECHO(HighParent,echo_high_parent) ECHO(DownParent,echo_down_parent)
ECHO(WideParent,echo_wide_parent) ECHO(FloatParent,echo_float_parent) ECHO(ArrayParent,echo_array_parent)
ECHO(PackedParent,echo_packed_parent) ECHO(PragmaParent,echo_pragma_parent) ECHO(MemberParent,echo_member_parent)
High shifted(int marker,High v) { v.value+=marker; return v; }
double overflow(double a,double b,double c,double d,double e,double f,double g,double h,FloatLow v,double tail) {
    return a+b+c+d+e+f+g+h+v.scalar+tail;
}
HighEntry high_factory(void) { return echo_high; }
High forward_high(HighEntry entry,High v) { return entry(v); }
High *high_pointer(High v) { static High storage; storage=v; return &storage; }
High read_high(High *p) { High v={0}; if(p) v=*p; return v; }
int high_alignment(High *p) { return (unsigned long long)p % _Alignof(High); }
long long wide_overflow(int a,int b,int c,int d,int e,int f,int g,int h,WideLow v,int tail) {
    return a+b+c+d+e+f+g+h+v.first+v.second+v.third+tail;
}
double alias_variadic(double a,double b,double c,double d,double e,double f,double g,double h,FloatLow v,...) {
    __builtin_va_list args; __builtin_va_start(args,v);
    int marker=__builtin_va_arg(args,int); double tail=__builtin_va_arg(args,double);
    __builtin_va_end(args); return a+b+c+d+e+f+g+h+v.scalar+marker+tail;
}
ECHO(SmallParent,echo_small_parent)
SmallParent shifted_small(int marker,SmallParent v) { v.small_child.value+=marker; return v; }
