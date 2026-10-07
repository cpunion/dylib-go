#include <stdint.h>
static int32_t bias = 7;
static int32_t zero;
int32_t state(int32_t a, int32_t b) { zero += a; return zero + b + bias; }
