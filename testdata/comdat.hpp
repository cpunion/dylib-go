#include <stdint.h>
inline int32_t comdat_increment(int32_t value) {
    static int32_t count = 0;
    return value + ++count;
}
