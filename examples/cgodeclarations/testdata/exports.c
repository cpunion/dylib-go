#include "exports.h"
signed char s8(signed char x){return x+1;}
unsigned char u8(unsigned char x){return x-1;}
short s16(short x){return x+1;}
unsigned short u16(unsigned short x){return x-1;}
int s32(int x){return x+1;}
unsigned int u32(unsigned int x){return x-1;}
long long s64(long long x){return x+1;}
unsigned long long u64(unsigned long long x){return x-1;}
float f32(float x){return x+1.5f;}
double f64(double x){return x+1.5;}
_Bool truth(_Bool x){return !x;}
void *pointer(void *x){return x;}
int datum = 42;
int zero(void){return 42;}
void empty(void){}
double mixed(int a,double b,float c,unsigned long long d){return a+b+c+d;}
int variable(int a, ...) {
    __builtin_va_list ap;
    __builtin_va_start(ap, a);
    int b = __builtin_va_arg(ap, int);
    double c = __builtin_va_arg(ap, double);
    __builtin_va_end(ap);
    return a + b + (int)c;
}
int promotions(int marker, ...) {
    __builtin_va_list ap;
    __builtin_va_start(ap, marker);
    int a = __builtin_va_arg(ap, int);
    int b = __builtin_va_arg(ap, int);
    int c = __builtin_va_arg(ap, int);
    int d = __builtin_va_arg(ap, int);
    double e = __builtin_va_arg(ap, double);
    int f = __builtin_va_arg(ap, int);
    long long g = __builtin_va_arg(ap, long long);
    unsigned long long h = __builtin_va_arg(ap, unsigned long long);
    void *p = __builtin_va_arg(ap, void *);
    __builtin_va_end(ap);
    if (marker != 7) return -1;
    if (a != -8) return -2;
    if (b != 250) return -3;
    if (c != -300) return -4;
    if (d != 60000) return -5;
    if (e != 11.5) return -6;
    if (f != 1) return -7;
    if (g != -9) return -8;
    if (h != 18446744073709551614ULL) return -9;
    if (p != &datum) return -10;
    return 42;
}
int empty_variable(int value, ...) { return value; }
int mixed_variable(float fixed, int marker, ...) {
    __builtin_va_list ap;
    __builtin_va_start(ap, marker);
    int a = __builtin_va_arg(ap, int);
    double b = __builtin_va_arg(ap, double);
    __builtin_va_end(ap);
    return marker == 7 ? (int)fixed + a + (int)b : -1;
}
void void_variable(int marker, ...) {
    __builtin_va_list ap;
    __builtin_va_start(ap, marker);
    datum = __builtin_va_arg(ap, int);
    __builtin_va_end(ap);
}
static int add(int a, int b) { return a + b; }
Adder adder_factory(void) { return add; }
int apply_adder(Adder entry, int a, int b) { return entry(a, b); }
Variadic variadic_factory(void) { return variable; }
int forward_variadic(Variadic entry, int a) { return entry(a, 10, 12.0); }
void *data_pointer(void) { return &datum; }
