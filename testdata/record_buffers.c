#include <stdint.h>
#if defined(_WIN32)
#define EXPORT __declspec(dllexport)
#else
#define EXPORT
#endif
struct pair { int32_t a, b; };
EXPORT int32_t reenter_record(int32_t (*fn)(int32_t), struct pair value,
                             struct pair *p, int32_t depth) {
    int32_t nested = fn(depth);
    p->a += depth;
    return nested + value.a + value.b + p->a + p->b;
}
