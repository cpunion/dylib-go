#include "exports.h"
#include <stdarg.h>

static int context;
static Packed stored = { -128, 20.5, 22, 1, 0 };

Packed echo_packed(Packed p) {
    p.tag += 1;
    p.value += 1.5;
    p.tail -= 1;
    p.truth = !p.truth;
    return p;
}
Envelope echo_envelope(Envelope p) {
    for (int i = 0; i < 2; ++i)
        for (int j = 0; j < 2; ++j)
            p.items[i][j] = echo_packed(p.items[i][j]);
    p.values[0] += 1;
    p.values[1] += 2;
    return p;
}
PackedFn packed_factory(void) { return echo_packed; }
Packed apply_packed(PackedFn fn, Packed p) { return fn(p); }
Packed *packed_pointer(void) { stored.context = &context; return &stored; }
Packed *mutate_packed(Packed *p) {
    if (p) *p = echo_packed(*p);
    return p;
}
double sum_packed(Packed *p) { return p ? p->value + p->tail : -1; }
Packed packed_variable(Packed p, ...) {
    va_list args;
    va_start(args, p);
    p.tail += va_arg(args, int);
    p.value += va_arg(args, double);
    va_end(args);
    return p;
}
