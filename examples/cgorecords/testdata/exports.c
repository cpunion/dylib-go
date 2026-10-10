#include "exports.h"
#include <stdarg.h>

Small echo_small(Small value) { value.a += 1; value.b -= 1; return value; }
Pair echo_pair(Pair value) { value.tag += 1; value.value += 1.5; value.tail -= 1; return value; }
Outer echo_outer(Outer value) {
    for (int i = 0; i < 2; ++i) {
        value.items[i] = echo_pair(value.items[i]);
        for (int j = 0; j < 3; ++j) value.values[i][j] += 1;
    }
    return value;
}
FloatVector echo_floats(FloatVector value) {
    for (int i = 0; i < 4; ++i) value.values[i] += 1.5f;
    return value;
}
Flags echo_flags(Flags value) {
    value.truth = !value.truth;
    value.u8 -= 1; value.u16 -= 1; value.s64 += 1; value.u64 -= 1;
    return value;
}
SmallFn small_factory(void) { return echo_small; }
Small apply_small(SmallFn entry, Small value) { return entry(value); }
OuterFn outer_factory(void) { return echo_outer; }
Outer apply_outer(OuterFn entry, Outer value) { return entry(value); }
Small record_variable(Small value, ...) {
    va_list arguments;
    va_start(arguments, value);
    value.a += va_arg(arguments, int);
    value.b += (int)va_arg(arguments, double);
    va_end(arguments);
    return value;
}
static int datum = 42;
void *data_pointer(void) { return &datum; }
