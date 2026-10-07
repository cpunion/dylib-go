#include <stdint.h>
#include <stdbool.h>
int8_t echo_i8(int8_t v) { return v; }
uint8_t echo_u8(uint8_t v) { return v; }
int16_t echo_i16(int16_t v) { return v; }
uint16_t echo_u16(uint16_t v) { return v; }
bool echo_bool(bool v) { return v; }
uint32_t echo_u32(uint32_t v) { return v; }
int64_t echo_i64(int64_t v) { return v; }
uint64_t echo_u64(uint64_t v) { return v; }
float echo_f32(float v) { return v; }
int32_t answer(void) { return 42; }
void no_result(void) {}

typedef struct { int32_t a, b; } pair;
int32_t sum_pair(pair v) { return v.a+v.b; }
int32_t sum_pair_ptr(const pair *v) { return v->a+v->b; }
pair echo_pair(pair v) { return v; }
pair *mutate_pair(pair *v) { v->a+=2; v->b-=2; return v; }
int32_t *interior_pair(pair *v) { return &v->b; }
bool same_pair(pair *a, pair *b) { a->a+=2; return a==b; }
int32_t scalar_ptr(int32_t *v) { *v+=2; return *v; }
typedef struct { int8_t tag; pair p; double extra; } nested;
double sum_nested(nested v) { return v.tag+v.p.a+v.p.b+v.extra; }
nested echo_nested(nested v) { return v; }
typedef struct { float x,y; } float_pair;
float_pair echo_float_pair(float_pair v) { return v; }
typedef struct { pair *p; int32_t bonus; } pair_ref;
pair_ref echo_pair_ref(pair_ref v) { v.p->a+=2; return v; }
