#include <stdarg.h>
#include <stdint.h>

double var_promotions(int32_t base, ...) {
    va_list ap;
    va_start(ap, base);
    int a = va_arg(ap, int);
    int b = va_arg(ap, int);
    int c = va_arg(ap, int);
    double d = va_arg(ap, double);
    double e = va_arg(ap, double);
    va_end(ap);
    return base + a + b + c + d + e;
}

double var_fixed(float fixed, int32_t count, ...) {
    va_list ap;
    va_start(ap, count);
    double result = fixed;
    for (int32_t i = 0; i < count; i++) result += va_arg(ap, double);
    va_end(ap);
    return result;
}

int32_t var_empty(int32_t value, ...) { return value; }

typedef struct { int32_t a, b; } Pair;
int32_t var_pair(int32_t extra, ...) {
    va_list ap;
    va_start(ap, extra);
    Pair p = va_arg(ap, Pair);
    va_end(ap);
    return p.a + p.b + extra;
}

int32_t var_pointer(int32_t extra, ...) {
    va_list ap;
    va_start(ap, extra);
    int32_t *p = va_arg(ap, int32_t *);
    va_end(ap);
    *p += 2;
    return *p + extra;
}
