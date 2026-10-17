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
union Nested nested_echo(union Nested value) {
    if (value.payload.before || value.payload.after) {
        union Nested invalid = {0};
        invalid.payload.inner.pair.a = -1;
        return invalid;
    }
    return value;
}
union Nested nested_add(union Nested value, int32_t add) {
    if (value.payload.before || value.payload.after || value.payload.inner.pair.b)
        value.payload.inner.pair.a = -1000;
    value.payload.inner.pair.a += add;
    return value;
}
union NestedSmall nested_small_add(union NestedSmall value, int32_t add) {
    if (value.payload.before) value.payload.inner.i = -1000;
    value.payload.inner.i += add;
    return value;
}
union Arrays arrays_make(int32_t a, int32_t b) { union Arrays v={0}; v.ints[0]=a; v.ints[1]=b; return v; }
union Arrays matrix_make(int32_t a, int32_t b) { union Arrays v={0}; v.matrix[0][0]=a; v.matrix[0][1]=b; v.matrix[1][0]=b; v.matrix[1][1]=a; return v; }
union Arrays pairs_make(int32_t a, int32_t b) { union Arrays v={0}; v.pairs[0].a=a; v.pairs[0].b=b; v.pairs[1].a=b; v.pairs[1].b=a; return v; }
union Arrays array_hfa_make(double a, double b) { union Arrays v={0}; v.doubles[0]=a; v.doubles[1]=b; return v; }
union Arrays pointers_make(void) { union Arrays v={0}; v.pointers[1]=&stored; return v; }
union Arrays arrays_echo(union Arrays value) { return value; }
union Arrays arrays_add(union Arrays value, int32_t add) { value.ints[0]+=add; value.ints[1]+=add; return value; }
union SmallArray small_array_add(union SmallArray value, int32_t add) { value.ints[0]+=add; value.ints[1]+=add; return value; }
union PackedArray packed_array_echo(union PackedArray value) { return value; }
union AlignedArray aligned_array_echo(union AlignedArray value) { return value; }
union NestedArray nested_array_echo(union NestedArray value) { if(value.payload.before || value.payload.after) value.payload.inner.ints[0]=-1000; return value; }
