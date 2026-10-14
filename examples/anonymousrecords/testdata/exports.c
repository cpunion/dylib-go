#include "exports.h"
#if defined(_MSC_VER)
int _fltused=0x9875;
#endif
#define ECHO(T,N) T N(T v){return v;}
ECHO(High,echo_high) ECHO(Inherited,echo_inherited) ECHO(Low,echo_low) ECHO(LowOnly,echo_low_only)
ECHO(Float,echo_float) ECHO(Wide,echo_wide) ECHO(WideLow,echo_wide_low) ECHO(Doubles,echo_doubles)
ECHO(Maximum,echo_maximum) ECHO(Packed,echo_packed) ECHO(Pragma,echo_pragma) ECHO(Member,echo_member)
ECHO(Bits,echo_bits) ECHO(Nested,echo_nested) ECHO(Array,echo_array) ECHO(Explicit,echo_explicit)
High shifted(int marker,High v){v.high_value+=marker;return v;}
#define PREFIX double a,double b,double c,double d,double e,double f,double g,double h
#define SUM (a+b+c+d+e+f+g+h)
double overflow(PREFIX,Float v,double tail){return SUM+v.scalar+tail;}
long long wide_overflow(int a,int b,int c,int d,int e,int f,int g,int h,WideLow v,int tail){return SUM+v.first+v.second+v.third+tail;}
double variadic(PREFIX,Float v,...){__builtin_va_list args;__builtin_va_start(args,v);int marker=__builtin_va_arg(args,int);double tail=__builtin_va_arg(args,double);__builtin_va_end(args);return SUM+v.scalar+marker+tail;}
HighEntry factory(void){return echo_high;} High forward(HighEntry entry,High v){return entry(v);}
High *pointer(High v){static High storage;storage=v;return &storage;}
/* A distinct export name avoids Windows CRT read import collisions. */
High read_high(High *p){High v={0};if(p)v=*p;return v;}
int alignment(High *p){return (unsigned long long)p % _Alignof(High);}
