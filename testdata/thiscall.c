#include "thiscall.h"
ThisState *this_state(void) { static ThisState s; s.bias=20; s.calls=0; return &s; }
int32_t this_calls(ThisState *s) { return s->calls; }
int32_t THIS this_add(ThisState *s, int32_t n) { ++s->calls; return s->bias+n; }
double THIS this_mixed(ThisState *s, int8_t a, uint16_t b, float c, double d, uint64_t e, int32_t f) {
    ++s->calls; return s->bias+a+b+c+d+e+f;
}
uint64_t THIS this_wide(ThisState *s, uint64_t n) { ++s->calls; return n^((uint64_t)s->bias<<48); }
void *THIS this_pointer(ThisState *s, void *p) { ++s->calls; return p; }
void THIS this_void(ThisState *s, int32_t n) { ++s->calls; s->bias=n; }
int32_t THIS this_payload(ThisState *s, ThisPayload p, int32_t n) { ++s->calls; return s->bias+p.a+(int32_t)p.b+p.c+n; }
ThisLarge THIS this_large(ThisState *s, int32_t n) { ThisLarge r={s->bias,n,20,22}; ++s->calls; return r; }
ThisEntry this_factory(void) { return this_mixed; }
double this_apply(ThisEntry entry, ThisState *s, int8_t a, uint16_t b, float c, double d, uint64_t e, int32_t f) { return entry(s,a,b,c,d,e,f); }
double THIS this_forward(ThisState *s, ThisEntry entry, int8_t a, uint16_t b, float c, double d, uint64_t e, int32_t f) { return entry(s,a,b,c,d,e,f); }

int32_t this_payload_apply(ThisPayloadEntry entry, ThisState *s, ThisPayload p, int32_t n) { return entry(s,p,n); }
