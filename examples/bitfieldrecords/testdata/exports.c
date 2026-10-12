#include "exports.h"
#include <stdarg.h>
// MSVC objects emit this CRT floating-point marker. Keep it in the fixture so
// the same producer object can also be linked with a MinGW driver.
#if defined(_MSC_VER)
int _fltused = 0x9875;
#endif
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
GapBits echo_gaps(GapBits p) {
    p.low ^= 31;
    p.high += 1;
    p.enabled = !p.enabled;
    p.value += 2;
    return p;
}
PrefixBits echo_prefix(PrefixBits p) { p.octet ^= 255; return p; }
PackedGapBits echo_packed_gaps(PackedGapBits p) {
    p.low ^= 31;
    p.high += 1;
    p.enabled = !p.enabled;
    p.real += 1.5;
    return p;
}
GapEnvelope echo_gap_envelope(GapEnvelope p) {
    for(int i=0;i<2;++i) {
        p.items[i]=echo_gaps(p.items[i]);
        p.packed[i]=echo_packed_gaps(p.packed[i]);
    }
    p.prefix=echo_prefix(p.prefix);
    return p;
}
static GapBits stored_gaps = {31,-64,1,20};
GapBits *gaps_pointer(void) { return &stored_gaps; }
GapBits *mutate_gaps(GapBits *p) { if(p) *p=echo_gaps(*p); return p; }
unsigned sum_gaps(GapBits *p) { return p ? p->value+p->low : 0; }
GapFn gaps_factory(void) { return echo_gaps; }
GapBits apply_gaps(GapFn fn, GapBits p) { return fn(p); }
GapBits gaps_variable(GapBits p, ...) {
    va_list args;
    va_start(args,p);
    p.high += va_arg(args,int);
    p.value += (unsigned)va_arg(args,double);
    va_end(args);
    return p;
}
FloatingGap echo_floating_gap(FloatingGap p) {
    p.first += 1.5;
    p.second += 2.5;
    return p;
}
MemberTiny echo_member_tiny(MemberTiny p) {
    p.low ^= 7;
    p.high += 1;
    return p;
}
MemberTrailing echo_member_trailing(MemberTrailing p) {
    p.code ^= 7;
    return p;
}
PackedZero echo_packed_zero(PackedZero p) {
    p.marker += 1;
    p.code ^= 7;
    p.tail += 2;
    return p;
}
