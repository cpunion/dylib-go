#include "exports.h"
#include <stdarg.h>
#include <stdint.h>

#if defined(_WIN32) && defined(_MSC_VER)
// The MSVC-targeted object is also linked by the GNU driver in native CI.
int _fltused = 0x9875;
#endif

static Aligned stored = { -128, 20.5, 22, 0 };

Aligned echo_aligned(Aligned p) { p.tag += 1; p.value += 1.5; p.tail += 2; return p; }
FieldAligned echo_fields(FieldAligned p) {
    p.tag += 1; p.values[0] += 1.5; p.values[1] += 2.5; p.count += 2;
    return p;
}
PackedAligned echo_packed(PackedAligned p) { p.tag += 1; p.value += 1.5; p.tail -= 1; return p; }
AlignedBits echo_bits(AlignedBits p) { p.tag += 1; p.delta += 1; p.value += 1.5; p.enabled ^= 1; return p; }
Envelope echo_envelope(Envelope p) {
    for (int i = 0; i < 2; ++i) {
        p.items[i] = echo_aligned(p.items[i]);
        p.fields[i] = echo_fields(p.fields[i]);
        p.packed[i] = echo_packed(p.packed[i]);
        p.bits[i] = echo_bits(p.bits[i]);
    }
    p.count += 1;
    return p;
}
AlignedFn aligned_factory(void) { return echo_aligned; }
Aligned apply_aligned(AlignedFn fn, Aligned p) { return fn(p); }
Aligned *aligned_pointer(void) { return &stored; }
Aligned *mutate_aligned(Aligned *p) { if (p) *p = echo_aligned(*p); return p; }
double sum_aligned(Aligned *p) { return p ? p->value + p->tail : -1; }
unsigned long long aligned_address(Aligned *p) { return (uintptr_t)p; }
Aligned aligned_variable(Aligned p, ...) {
    va_list args;
    va_start(args, p);
    p.tail += va_arg(args, int);
    p.value += va_arg(args, double);
    va_end(args);
    return p;
}
AlignedFloats echo_floats(AlignedFloats p) { p.first += 1.5; p.second += 2.5; return p; }
FieldAlignedBits echo_field_bits(FieldAlignedBits p) { p.low += 1; p.value += 2; p.high += 1; p.tag += 1; return p; }
PackedFields echo_packed_fields(PackedFields p) { p.tag += 1; p.values[0] += 1.5; p.values[1] += 2.5; p.tail -= 1; return p; }
PackedFieldBits echo_packed_field_bits(PackedFieldBits p) { p.lowbits += 1; p.counter += 2; p.highbits += 1; p.marker += 1; return p; }
Aligned ALIGNED_STDCALL echo_stdcall(Aligned p, int delta) { p.tail += delta; return p; }
Aligned ALIGNED_FASTCALL echo_fastcall(Aligned p, int delta, Aligned q, int extra) { p.value += q.value; p.tail += delta + extra; return p; }
AlignedStdFn stdcall_factory(void) { return echo_stdcall; }
AlignedFastFn fastcall_factory(void) { return echo_fastcall; }
Aligned apply_stdcall(AlignedStdFn fn, Aligned p, int delta) { return fn(p, delta); }
Aligned apply_fastcall(AlignedFastFn fn, Aligned p, int delta, Aligned q, int extra) { return fn(p, delta, q, extra); }
