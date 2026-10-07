#include <stdint.h>
int32_t __attribute__((stdcall)) stdcall_add(int32_t a, int32_t b) { return a + b; }
int32_t __attribute__((fastcall)) fastcall_add(int32_t a, int32_t b) { return a + b; }
