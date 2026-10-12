#include "exports.h"
TestEnumI8 s8(TestEnumI8 x){return x+1;}
TestEnumU8 u8(TestEnumU8 x){return x-1;}
TestEnumI16 s16(TestEnumI16 x){return x+1;}
TestEnumU16 u16(TestEnumU16 x){return x-1;}
TestEnumI32 s32(TestEnumI32 x){return x+1;}
TestEnumU32 u32(TestEnumU32 x){return x-1;}
TestEnumI64 s64(TestEnumI64 x){return x+1;}
TestEnumU64 u64(TestEnumU64 x){return x-1;}
float f32(float x){return x+1.5f;}
double f64(double x){return x+1.5;}
TestEnumBool truth(TestEnumBool x){return !x;}
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

Maker maker_factory(void) { return adder_factory; }
MetaMaker meta_factory(void) { return maker_factory; }
Consumer consumer_factory(void) { return apply_adder; }
static int subtract(int a, int b) { return a - b; }
Adder subtract_factory(void) { return subtract; }
int apply_maker(Maker maker, int a, int b) { return maker()(a, b); }
int apply_meta(MetaMaker maker, int a, int b) { return maker()()(a, b); }
int apply_consumer(Consumer consumer, Adder entry, int a, int b) { return consumer(entry, a, b); }
static Adder entry_slot = add;
Adder *adder_slot(void) { return &entry_slot; }
void replace_adder(Adder *slot, Adder entry) { *slot = entry; }
void *data_pointer(void) { return &datum; }
