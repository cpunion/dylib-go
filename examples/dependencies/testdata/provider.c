#include <stdint.h>
#if defined(_WIN32)
#define EXPORT __declspec(dllexport)
#else
#define EXPORT
#endif
EXPORT int32_t dependency_bias = 2;
EXPORT int32_t dependency_add(int32_t a, int32_t b) { return a + b; }
