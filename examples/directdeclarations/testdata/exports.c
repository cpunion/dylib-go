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
