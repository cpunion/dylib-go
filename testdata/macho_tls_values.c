#include <stdint.h>

static int ordinary = 22;
static _Thread_local struct { int64_t n; double f; } record = {20, 1.25};
static _Thread_local int *pointer = &ordinary;
static _Thread_local char bytes[5] = {1, 2, 3, 4, 5};
static _Thread_local int zero;

int native_tls_values(int a, int b) {
    if (pointer != &ordinary || *pointer != 22 || bytes[2] != 3 || bytes[4] != 5 ||
        record.n != 20+zero || record.f != 1.25+zero) return -1;
    int result = (int)record.n + a+b;
    ++zero;
    ++record.n;
    record.f += 1;
    return result;
}

struct big { int64_t a, b; double f; };
// Keep integer/FP arguments live across native TLS lookups, including an
// indirect result and stack arguments. Inlining cannot replace the TLV call.
__attribute__((noinline)) struct big native_tls_mixed_record(struct big value,
    double f0,double f1,double f2,double f3,double f4,double f5,double f6,double f7,double f8,double f9,
    int n0,int n1,int n2,int n3,int n4,int n5,int n6,int n7,int n8,int n9) {
    value.a += record.n+n0+n1+n2+n3+n4+n5+n6+n7+n8+n9;
    value.b += *pointer;
    value.f += record.f+f0+f1+f2+f3+f4+f5+f6+f7+f8+f9;
    return value;
}

int native_tls_mixed(int a, int b) {
    struct big input = {a,b,0.5};
    struct big actual = native_tls_mixed_record(input, 0,1,2,3,4,5,6,7,8,9, 0,1,2,3,4,5,6,7,8,9);
    if (actual.a != a+record.n+45 || actual.b != b+22 || actual.f != 45.5+record.f) return -1;
    return 42;
}
