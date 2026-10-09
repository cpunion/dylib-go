#include <stdint.h>
#include <stddef.h>
#if defined(_WIN32)
#define EXPORT __declspec(dllexport)
#else
#define EXPORT
#endif
struct pair { int32_t a, b; };
struct big { int64_t a, b, c; };
struct padded { uint8_t tag; uint64_t value; };
struct wide { uint64_t a, b, c, d; };
EXPORT int32_t fill_wide(struct wide *value) {
    volatile unsigned char *bytes = (volatile unsigned char *)value;
    for (size_t i = 0; i < sizeof(*value); i++) bytes[i] = 0xa5;
    return 42;
}
EXPORT int32_t check_padded(struct padded *value) {
    const unsigned char *bytes = (const unsigned char *)value;
    for (size_t i = 1; i < offsetof(struct padded, value); i++) {
        if (bytes[i] != 0) return -1;
    }
    if (value->tag != 20 || value->value != 22) return -2;
    value->tag = 22;
    value->value = 20;
    return 42;
}
EXPORT struct padded *alias_padded(struct padded *left, struct padded *right) {
    if (left != right) return NULL;
    left->value += 2;
    return right;
}
EXPORT struct big bump_big(struct big value, int64_t delta) {
    value.a += delta;
    return value;
}
EXPORT int32_t reenter_record(int32_t (*fn)(int32_t), struct pair value,
                             struct pair *p, int32_t depth) {
    int32_t nested = fn(depth);
    p->a += depth;
    return nested + value.a + value.b + p->a + p->b;
}
