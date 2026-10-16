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
#endif
