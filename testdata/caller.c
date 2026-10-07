#include <stdint.h>
extern int32_t add(int32_t, int32_t);
int32_t caller(int32_t a, int32_t b) { return add(a, b) + 1; }
