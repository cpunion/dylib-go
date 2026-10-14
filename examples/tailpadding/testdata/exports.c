#include "exports.h"
#if defined(_MSC_VER)
int _fltused=0x9875;
#endif
#define ECHO(T,N) T N(T v){return v;}
ECHO(Float,echo_float) ECHO(Double,echo_double) ECHO(Integer,echo_integer)
ECHO(Array,echo_array) ECHO(Triple,echo_triple) ECHO(Parent,echo_parent)
#define PREFIX double a,double b,double c,double d,double e,double f,double g,double h
#define SUM (a+b+c+d+e+f+g+h)
double float_overflow(PREFIX,Float v,double tail){return SUM+v.float_value+tail;}
double double_overflow(PREFIX,Double v,double tail){return SUM+v.double_value+tail;}
long long integer_overflow(int a,int b,int c,int d,int e,int f,int g,int h,Integer v,int tail){return SUM+v.integer_value+tail;}
#define VAR(T,N,FIELD) double N(PREFIX,T v,...){__builtin_va_list ap;__builtin_va_start(ap,v);int i=__builtin_va_arg(ap,int);double x=__builtin_va_arg(ap,double);__builtin_va_end(ap);return SUM+v.FIELD+i+x;}
VAR(Float,float_variadic,float_value) VAR(Double,double_variadic,double_value)
FloatEntry float_factory(void){return echo_float;}
Float forward_float(FloatEntry entry,Float v){return entry(v);}
static Double storage;
Double *double_pointer(Double v){storage=v;return &storage;}
Double *mutate_double(Double *v){if(v){v->double_value+=2;}return v;}
Double read_double(Double *v){return *v;}
