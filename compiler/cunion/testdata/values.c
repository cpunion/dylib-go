#include "values.h"
static int32_t stored;
a0 target(a0 value) { return value; }
union Number number_add(union Number a, union Number b) { union Number result = {0}; result.i=a.i+b.i; return result; }
union Small small_add(union Small value, float add) { value.f+=add; return value; }
union Large large_add(union Large value, int32_t add) { value.i+=add; return value; }
union Packed packed_add(union Packed value, double add) { value.d+=add; return value; }
union Aligned aligned_add(union Aligned value, int32_t add) { value.i+=add; return value; }
void store_number(union Number value) { stored=value.i; }
int32_t read_number(union Number value) { return value.i; }
int32_t stored_number(void) { return stored; }
void *native_address(void) { return &stored; }
int32_t read_pointer(union Number value) { return *(int32_t *)value.p; }

// Raw objects resolve compiler-generated aggregate copies locally. Volatile
// accesses keep these fixture-only helpers from becoming recursive libc calls.
#include <stddef.h>
void *memcpy(void *destination, const void *source, size_t size) {
    volatile unsigned char *out = destination;
    const volatile unsigned char *in = source;
    for (size_t i = 0; i < size; ++i) out[i] = in[i];
    return destination;
}
void *memset(void *destination, int value, size_t size) {
    volatile unsigned char *out = destination;
    for (size_t i = 0; i < size; ++i) out[i] = (unsigned char)value;
    return destination;
}
struct Pair pair_make(int32_t a, int32_t b) { struct Pair p = {a,b}; return p; }
union Records record_pair_add(union Records value, struct Pair add) { value.pair.a+=add.a; value.pair.b+=add.b; return value; }
struct Bundle bundle_make(int32_t a, int32_t b) { struct Bundle v = {{a,b}, {{a,b},{b,a}}, {20.5,21.5}, &stored}; return v; }
union Records record_bundle_echo(union Records value) { return value; }

struct HFA hfa_make(double a,double b) { struct HFA v={{a,b}}; return v; }
union Records record_hfa_echo(union Records value) { return value; }
