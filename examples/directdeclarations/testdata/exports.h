#include "../../../testdata/enum_types.h"
TestEnumI8 s8(TestEnumI8);
TestEnumU8 u8(TestEnumU8);
TestEnumI16 s16(TestEnumI16);
TestEnumU16 u16(TestEnumU16);
TestEnumI32 s32(TestEnumI32);
TestEnumU32 u32(TestEnumU32);
TestEnumI64 s64(TestEnumI64);
TestEnumU64 u64(TestEnumU64);
float f32(float);
double f64(double);
TestEnumBool truth(TestEnumBool);
void *pointer(void *);
int zero(void);
void empty(void);
double mixed(int, double, float, unsigned long long);
typedef struct {TestEnumI8 tag;double value;TestEnumI16 tail;} Pair;
Pair echo_pair(Pair);
Pair *mutate_pair(Pair *);
typedef struct {Pair items[2];int values[2][3];void *next;} Outer;
Outer echo_outer(Outer);
typedef struct {float values[4];} FloatVector;
FloatVector echo_floats(FloatVector);

typedef struct { int a, b; } Small;
Small echo_small(Small);
typedef struct { double value; int tag; } MixedPair;
MixedPair echo_mixed(MixedPair);

typedef TestEnumI32 (*Adder)(TestEnumI32, TestEnumI32);
Adder adder_factory(void);
int apply_adder(Adder, int, int);
int apply_anonymous(int (*)(int, int), int, int);
int apply_decayed(int callback(int, int), int, int);
typedef Small (*SmallFn)(Small);
SmallFn small_factory(void);
Small apply_small(SmallFn, Small);
typedef Outer (*OuterFn)(Outer);
OuterFn outer_factory(void);
Outer apply_outer(OuterFn, Outer);
typedef Pair *(*Mutator)(Pair *);
Mutator mutator_factory(void);
Pair *apply_mutator(Mutator, Pair *);

int variable(int, ...);
int promotions(int, ...);
int empty_variable(int, ...);
double mixed_variable(float, int, ...);
void void_variable(int *, ...);
int *data_pointer(void);
double many_variable(int, ...);
Small record_variable(Small, ...);
typedef int (*Variadic)(int, ...);
Variadic variadic_factory(void);
int forward_variadic(Variadic);

int narrow(signed char, unsigned char, short, unsigned short, _Bool);
typedef int (*Narrow)(signed char, unsigned char, short, unsigned short, _Bool);
Narrow narrow_factory(void);
int apply_narrow(Narrow, signed char, unsigned char, short, unsigned short, _Bool);
Outer narrow_stack(int, int, int, int, int, int, int, int,
                   signed char, unsigned char, short, unsigned short, _Bool);
int narrow_variable(signed char, unsigned char, short, unsigned short, _Bool, int, ...);
typedef int (*NarrowVariadic)(signed char, unsigned char, short, unsigned short, _Bool, int, ...);
NarrowVariadic narrow_variadic_factory(void);
int forward_narrow_variadic(NarrowVariadic);

typedef Adder (*Maker)(void);
typedef Maker (*MetaMaker)(void);
typedef int (*Consumer)(Adder, int, int);
Maker maker_factory(void);
MetaMaker meta_factory(void);
Consumer consumer_factory(void);
Adder subtract_factory(void);
int apply_maker(Maker, int, int);
int apply_meta(MetaMaker, int, int);
int apply_consumer(Consumer, Adder, int, int);
Adder *adder_slot(void);
void replace_adder(Adder *, Adder);
