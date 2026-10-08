#include <stdint.h>
double mixed(int32_t a, double b, float c, uint64_t d) { return a+b+c+d; }
int32_t negate(int32_t a) { return -a; }
uint64_t wide(uint64_t a) { return a ^ UINT64_C(0xfeed123456789abc); }
void *echo_ptr(void *p) { return p; }
int32_t callback_add(int32_t (*callback)(int32_t, int32_t), int32_t a, int32_t b) {
    return callback(a, b);
}
