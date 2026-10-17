#ifndef DYLIB_GO_CUNION_VALUES_H
#define DYLIB_GO_CUNION_VALUES_H
#include <stdint.h>
#include <stdbool.h>
union Number { int8_t i8; uint8_t u8; int16_t i16; uint16_t u16; int32_t i; uint32_t u; int64_t i64; uint64_t u64; float f; double d; bool b; void *p; };
typedef union Number a0;
union Small { float f; uint32_t bits; };
union Large { int32_t i; double d; uint8_t storage[64]; };
union __attribute__((packed)) Packed { int32_t i; double d; };
union __attribute__((aligned(32))) Aligned { int32_t i; double d; };
a0 target(a0 value);
union Number number_add(union Number a, union Number b);
union Small small_add(union Small value, float add);
union Large large_add(union Large value, int32_t add);
union Packed packed_add(union Packed value, double add);
union Aligned aligned_add(union Aligned value, int32_t add);
void store_number(union Number value);
int32_t read_number(union Number value);
int32_t stored_number(void);
void *native_address(void);
int32_t read_pointer(union Number value);
struct Pair { int32_t a, b; };
struct Bundle { struct Pair pair; int32_t rows[2][2]; double values[2]; void *pointer; };
struct HFA { double values[2]; };
union Records { struct HFA hfa; struct Pair pair; struct Bundle bundle; unsigned char storage[128]; };
struct Pair pair_make(int32_t a, int32_t b);
union Records record_pair_add(union Records value, struct Pair add);
struct Bundle bundle_make(int32_t a, int32_t b);
union Records record_bundle_echo(union Records value);

struct HFA hfa_make(double a,double b);
union Records record_hfa_echo(union Records value);
union Nested {
    struct { int32_t before; union Records inner; union Number number; int32_t after; } payload;
    unsigned char storage[256];
};
union Nested nested_echo(union Nested value);
union Nested nested_add(union Nested value, int32_t add);
union NestedSmall {
    struct { int32_t before; union { int32_t i; float f; } inner; } payload;
    double alternative;
};
union NestedSmall nested_small_add(union NestedSmall value, int32_t add);
typedef int32_t IntPair[2];
typedef int32_t Matrix[2][2];
typedef struct Pair PairArray[2];
typedef double DoublePair[2];
typedef void *PointerPair[2];
union Arrays { IntPair ints; Matrix matrix; PairArray pairs; DoublePair doubles; PointerPair pointers; unsigned char storage[128]; };
union Arrays arrays_make(int32_t a, int32_t b);
union Arrays matrix_make(int32_t a, int32_t b);
union Arrays pairs_make(int32_t a, int32_t b);
union Arrays array_hfa_make(double a, double b);
union Arrays pointers_make(void);
union Arrays arrays_echo(union Arrays value);
union Arrays arrays_add(union Arrays value, int32_t add);
union SmallArray { IntPair ints; double alternative; };
union SmallArray small_array_add(union SmallArray value, int32_t add);
union __attribute__((packed)) PackedArray { DoublePair doubles; unsigned char storage[17]; };
union PackedArray packed_array_echo(union PackedArray value);
union __attribute__((aligned(32))) AlignedArray { IntPair ints; };
union AlignedArray aligned_array_echo(union AlignedArray value);
union NestedArray { struct { int32_t before; union Arrays inner; int32_t after; } payload; };
union NestedArray nested_array_echo(union NestedArray value);
#endif
