#include "exports.h"
// Keep raw objects independent of compiler-runtime aggregate copy helpers.
void *memcpy(void *dst, const void *src, __SIZE_TYPE__ n) {
    unsigned char *d = dst; const unsigned char *s = src;
    while (n--) *d++ = *s++;
    return dst;
}
static int stored;
unsigned VECTOR vector_unsigned(unsigned x) { return x - 1; }
_Bool VECTOR vector_bool(_Bool x) { return !x; }
void VECTOR vector_void(void) { stored = 42; }
int VECTOR vector_zero(void) { return stored; }
int VECTOR _vector_add(int a, int b) { return a + b + 1; }
int _cdecl_add(int a, int b) { return a + b - 1; }
int VECTOR vector_add(int a, int b) { return a + b; }
long long VECTOR vector_wide(signed char a, unsigned char b, short c, unsigned short d, long long e, unsigned long long f) {
    return a + b + c + d + e + (long long)f;
}
double VECTOR vector_mixed(int a, float b, double c, int d, float e, double f, float g, double h, int i) {
    return a + b + c + d + e + f + g + h + i;
}
Pair VECTOR vector_pair(Pair p) { p.a += 1; p.b -= 1; return p; }
HFA VECTOR vector_hfa(HFA p) { p.a += 1; p.b += 2; p.c += 3; p.d += 4; return p; }
Large VECTOR vector_large(Large p) { p.a[0] += 1; p.a[7] -= 1; return p; }
void * VECTOR vector_pointer(void *p) { return p; }
Entry vector_factory(void) { return vector_add; }
int vector_forward(Entry entry, int a, int b) { return entry(a, b); }
int VECTOR vector_apply(Entry entry, int a, int b) { return entry(a, b); }
HFAEntry vector_hfa_factory(void) { return vector_hfa; }
HFA vector_hfa_forward(HFAEntry entry, HFA value) { return entry(value); }
