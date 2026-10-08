/* Large fixed signatures exercise register-to-stack transitions and order. */
#include <stdint.h>
#include <stdarg.h>
#if defined(_WIN32)
#define EXPORT __declspec(dllexport)
#else
#define EXPORT
#endif
#define PARAMETERS(T) \
    T a0, T a1, T a2, T a3, T a4, T a5, T a6, T a7, \
    T a8, T a9, T a10, T a11, T a12, T a13, T a14, T a15, \
    T a16, T a17, T a18, T a19, T a20, T a21, T a22, T a23, \
    T a24, T a25, T a26, T a27, T a28, T a29, T a30, T a31, \
    T a32, T a33, T a34, T a35, T a36, T a37, T a38, T a39, \
    T a40, T a41, T a42, T a43, T a44, T a45, T a46, T a47, \
    T a48, T a49, T a50, T a51, T a52, T a53, T a54, T a55, \
    T a56, T a57, T a58, T a59, T a60, T a61, T a62, T a63
#define ARGUMENTS \
    a0, a1, a2, a3, a4, a5, a6, a7, \
    a8, a9, a10, a11, a12, a13, a14, a15, \
    a16, a17, a18, a19, a20, a21, a22, a23, \
    a24, a25, a26, a27, a28, a29, a30, a31, \
    a32, a33, a34, a35, a36, a37, a38, a39, \
    a40, a41, a42, a43, a44, a45, a46, a47, \
    a48, a49, a50, a51, a52, a53, a54, a55, \
    a56, a57, a58, a59, a60, a61, a62, a63
#define VALUES \
    1, 2, 3, 4, 5, 6, 7, 8, \
    9, 10, 11, 12, 13, 14, 15, 16, \
    17, 18, 19, 20, 21, 22, 23, 24, \
    25, 26, 27, 28, 29, 30, 31, 32, \
    33, 34, 35, 36, 37, 38, 39, 40, \
    41, 42, 43, 44, 45, 46, 47, 48, \
    49, 50, 51, 52, 53, 54, 55, 56, \
    57, 58, 59, 60, 61, 62, 63, 64

typedef struct { int32_t a, b; } pair;
EXPORT int32_t sum_two(int32_t a, int32_t b) { return a+b; }
EXPORT int32_t sub_two(int32_t a, int32_t b) { return a-b; }
EXPORT int32_t sum_pair(pair v) { return v.a+v.b; }
EXPORT int32_t read_pair(pair *v) { return v->a+v->b; }
EXPORT void *identity_pointer(void *p) { return p; }
EXPORT int64_t sum_many(PARAMETERS(int32_t)) {
    int32_t values[] = { ARGUMENTS };
    int64_t sum = 0;
    for (int i=0; i<64; i++) sum += (i+1)*(int64_t)values[i];
    return sum;
}
EXPORT int64_t sum_many_pairs(PARAMETERS(pair)) {
    pair values[] = { ARGUMENTS };
    int64_t sum = 0;
    for (int i=0; i<64; i++) sum += (i+1)*(3*(int64_t)values[i].a+values[i].b);
    return sum;
}
typedef int64_t (*many_callback)(PARAMETERS(int32_t));
EXPORT int64_t invoke_many(many_callback fn) { return fn(VALUES); }

/* Tails alternate promoted float32, int8 and uint16 values. */
EXPORT double sum_many_variadic(int32_t n, ...) {
    va_list ap;
    va_start(ap, n);
    double sum = 0;
    for (int32_t i=0; i<n; i++) {
        double value = i%3 == 0 ? va_arg(ap, double) : va_arg(ap, int);
        sum += (i+1)*value;
    }
    va_end(ap);
    return sum;
}
EXPORT int32_t answer(void) { return 42; }
