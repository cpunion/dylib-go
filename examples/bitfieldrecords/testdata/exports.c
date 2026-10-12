#include "exports.h"
#include <stdarg.h>
static int context;
static Bits stored = {7,-8,1,1099511627775ULL,20.5,0};
Bits echo_bits(Bits p) {
    p.flags ^= 7;
    p.delta += 1;
    p.active = !p.active;
    p.wide -= 1;
    p.value += 1.5;
    return p;
}
PackedBits echo_packed_bits(PackedBits p) {
    p.a ^= 7;
    p.b += 1;
    p.on = !p.on;
    p.big -= 1;
    p.real += 1.5;
    return p;
}
Envelope echo_envelope(Envelope p) {
    p.regular = echo_bits(p.regular);
    for(int i=0;i<2;++i) for(int j=0;j<2;++j)
        p.packed[i][j] = echo_packed_bits(p.packed[i][j]);
    return p;
}
BitsFn bits_factory(void) { return echo_bits; }
Bits apply_bits(BitsFn fn, Bits p) { return fn(p); }
Bits *bits_pointer(void) { stored.context=&context; return &stored; }
Bits *mutate_bits(Bits *p) { if(p) *p=echo_bits(*p); return p; }
double sum_bits(Bits *p) { return p ? p->value+p->delta : -1; }
Bits bits_variable(Bits p, ...) {
    va_list args;
    va_start(args,p);
    p.delta += va_arg(args,int);
    p.value += va_arg(args,double);
    va_end(args);
    return p;
}
SignedWide echo_signed_wide(SignedWide p) {
    p.value += 1;
    p.all ^= ~0ULL;
    return p;
}
