#include <stdint.h>
#include <stdarg.h>
#if defined(_WIN32)
#define EXPORT __declspec(dllexport)
#else
#define EXPORT
#endif

typedef struct { int32_t values[2]; } array_i32;
typedef struct { float values[4]; } array_f32;
typedef struct { double values[2]; } array_f64;
typedef struct { int8_t tag; int16_t values[3]; double extra; } array_padded;
typedef struct { int8_t tag; double value; } array_item;
typedef struct { array_item items[2]; int32_t tail; } array_records;
typedef struct { int32_t values[2][3]; } array_matrix;
typedef struct { uint8_t values[40]; } array_bytes;

// Each checksum observes the compiler's actual C layout. The round trips alone
// could hide a shared mistake in call and callback marshaling.
EXPORT double sum_array_i32(array_i32 v) { return v.values[0]+v.values[1]; }
EXPORT double sum_array_f32(array_f32 v) { return v.values[0]+v.values[1]+v.values[2]+v.values[3]; }
EXPORT double sum_array_f64(array_f64 v) { return v.values[0]+v.values[1]; }
EXPORT double sum_array_padded(array_padded v) { return v.tag+v.values[0]+v.values[1]+v.values[2]+v.extra; }
EXPORT double sum_array_records(array_records v) { return v.items[0].tag+v.items[0].value+v.items[1].tag+v.items[1].value+v.tail; }
EXPORT double sum_array_matrix(array_matrix v) { return v.values[0][0]+v.values[0][1]+v.values[0][2]+v.values[1][0]+v.values[1][1]+v.values[1][2]; }
EXPORT double sum_array_bytes(array_bytes v) { double sum=0; for (unsigned i=0;i<40;i++) sum+=v.values[i]; return sum; }

#define ARRAY_CASE(type, ...) \
    EXPORT type echo_##type(type v) { return v; } \
    EXPORT double callback_##type(type (*fn)(type)) { type v = __VA_ARGS__; return sum_##type(fn(v)); }
ARRAY_CASE(array_i32, {{20,22}})
ARRAY_CASE(array_f32, {{10.5f,10.5f,10.5f,10.5f}})
ARRAY_CASE(array_f64, {{20.5,21.5}})
ARRAY_CASE(array_padded, {1,{10,11,19},1})
ARRAY_CASE(array_records, {{{1,10},{2,20}},9})
ARRAY_CASE(array_matrix, {{{1,2,3},{10,11,15}}})
ARRAY_CASE(array_bytes, {{20,22}})

EXPORT int32_t var_array(int32_t base, ...) {
    va_list args;
    va_start(args,base);
    array_i32 v=va_arg(args,array_i32);
    va_end(args);
    return base+v.values[0]+v.values[1];
}

EXPORT int32_t sum_array_ptr(const int32_t (*v)[2]) { return (*v)[0]+(*v)[1]; }
EXPORT int32_t (*mutate_array(int32_t (*v)[2]))[2] { (*v)[0]+=2; (*v)[1]-=2; return v; }
EXPORT int32_t same_array(int32_t (*a)[2], int32_t (*b)[2]) { (*a)[0]+=2; return a==b; }
EXPORT int32_t *interior_array(int32_t (*v)[2]) { return &(*v)[1]; }
EXPORT int32_t *first_array(int32_t (*v)[2]) { return &(*v)[0]; }
typedef struct { int32_t *values[2]; } array_refs;
EXPORT array_refs mutate_array_refs(array_refs v) { *v.values[0]+=2; *v.values[1]-=2; return v; }
EXPORT int32_t callback_array_refs(array_refs (*fn)(array_refs)) {
    int32_t a=20,b=22;
    array_refs v={{&a,&b}}, out=fn(v);
    return out.values[0]==&a && out.values[1]==&b ? *out.values[0]+*out.values[1] : -1;
}
