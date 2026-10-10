#include <stdint.h>
#if defined(_WIN32)
#define IMPORT __declspec(dllimport)
#else
#define IMPORT
#endif
IMPORT extern int32_t dependency_bias;
IMPORT extern int32_t dependency_add(int32_t, int32_t);
int32_t imported_add(int32_t a, int32_t b) {
    return dependency_add(a, b) + dependency_bias;
}
