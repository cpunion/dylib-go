#include "exports.h"
#include <stdarg.h>
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
int zero(void){return 42;}
void empty(void){}
double mixed(int a,double b,float c,unsigned long long d){return a+b+c+d;}
Pair echo_pair(Pair pair){return pair;}
Pair *mutate_pair(Pair *pair){pair->tag++;pair->value++;pair->tail++;return pair;}
Outer echo_outer(Outer value){return value;}
FloatVector echo_floats(FloatVector value){return value;}

Small echo_small(Small value) { return value; }
MixedPair echo_mixed(MixedPair value) { return value; }

static int add(int a, int b) { return a + b; }
Adder adder_factory(void) { return add; }
int apply_adder(Adder callback, int a, int b) { return callback(a, b); }
int apply_anonymous(int (*callback)(int, int), int a, int b) { return callback(a, b); }
int apply_decayed(int callback(int, int), int a, int b) { return callback(a, b); }
SmallFn small_factory(void) { return echo_small; }
Small apply_small(SmallFn callback, Small value) { return callback(value); }
OuterFn outer_factory(void) { return echo_outer; }
Outer apply_outer(OuterFn callback, Outer value) { return callback(value); }
Mutator mutator_factory(void) { return mutate_pair; }
Pair *apply_mutator(Mutator callback, Pair *value) { return callback(value); }

int variable(int base, ...) {
    va_list ap;
    va_start(ap, base);
    int integer = va_arg(ap, int);
    double number = va_arg(ap, double);
    va_end(ap);
    return base + integer + (int)number;
}

int promotions(int marker, ...) {
    va_list ap;
    va_start(ap, marker);
    int signed8 = va_arg(ap, int), unsigned8 = va_arg(ap, int);
    int signed16 = va_arg(ap, int), unsigned16 = va_arg(ap, int);
    double number = va_arg(ap, double);
    int truth = va_arg(ap, int), falsity = va_arg(ap, int);
    long long wide = va_arg(ap, long long);
    unsigned long long unsigned_wide = va_arg(ap, unsigned long long);
    void *pointer = va_arg(ap, void *);
    va_end(ap);
    return marker == 7 && signed8 == -8 && unsigned8 == 255 && signed16 == -300 &&
        unsigned16 == 65535 && number == 20.5 && truth == 1 && falsity == 0 &&
        wide == -9 && unsigned_wide == 18446744073709551615ULL && pointer == 0 ? 42 : -1;
}

int empty_variable(int marker, ...) { return marker; }
double mixed_variable(float fixed, int base, ...) {
    va_list ap;
    va_start(ap, base);
    int integer = va_arg(ap, int);
    double number = va_arg(ap, double);
    va_end(ap);
    return fixed + base + integer + number;
}

static int data;
int *data_pointer(void) { return &data; }
void void_variable(int *output, ...) {
    va_list ap;
    va_start(ap, output);
    *output = va_arg(ap, int);
    va_end(ap);
}

double many_variable(int count, ...) {
    va_list ap;
    va_start(ap, count);
    double sum = 0;
    for (int i = 0; i < count; i++) {
        long long integer = va_arg(ap, long long);
        double number = va_arg(ap, double);
        sum += integer + number;
    }
    va_end(ap);
    return sum;
}

Small record_variable(Small value, ...) {
    va_list ap;
    va_start(ap, value);
    int integer = va_arg(ap, int);
    double number = va_arg(ap, double);
    va_end(ap);
    value.a += integer;
    value.b += (int)number;
    return value;
}

Variadic variadic_factory(void) { return variable; }
int forward_variadic(Variadic entry) { return entry(20, 10, 12.0); }

/* Wide results expose incorrect argument extension that narrow returns hide. */
int narrow(signed char a, unsigned char b, short c, unsigned short d, _Bool flag) {
    return 3 * a + b + 5 * c + d + (flag ? 101 : 0);
}
Narrow narrow_factory(void) { return narrow; }
int apply_narrow(Narrow entry, signed char a, unsigned char b, short c,
                 unsigned short d, _Bool flag) {
    return entry(a, b, c, d, flag);
}
Outer narrow_stack(int p0, int p1, int p2, int p3, int p4, int p5, int p6, int p7,
                   signed char a, unsigned char b, short c, unsigned short d, _Bool flag) {
    Outer result = {0};
    result.values[0][0] = narrow(a, b, c, d, flag);
    result.values[0][1] = p0 + 2*p1 + 3*p2 + 4*p3 + 5*p4 + 6*p5 + 7*p6 + 8*p7;
    return result;
}
int narrow_variable(signed char a, unsigned char b, short c, unsigned short d,
                    _Bool flag, int count, ...) {
    int result = narrow(a, b, c, d, flag);
    if (count) {
        va_list ap;
        va_start(ap, count);
        result += va_arg(ap, int);
        result += (int)va_arg(ap, double);
        va_end(ap);
    }
    return result;
}
NarrowVariadic narrow_variadic_factory(void) { return narrow_variable; }

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

int forward_narrow_variadic(NarrowVariadic entry) {
    return entry(-8, 255, -300, 65535, 1, 2, -7, 12.0);
}
