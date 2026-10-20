#ifndef DYLIB_GO_TEST_THISCALL_H
#define DYLIB_GO_TEST_THISCALL_H
#include <stdint.h>
#define THIS __attribute__((thiscall))
typedef struct { int32_t bias, calls; } ThisState;
typedef struct { int32_t a; double b; int32_t c; } ThisPayload;
typedef struct { int32_t a, b, c, d; } ThisLarge;
ThisState *this_state(void);
int32_t this_calls(ThisState *);
int32_t THIS this_add(ThisState *, int32_t);
double THIS this_mixed(ThisState *, int8_t, uint16_t, float, double, uint64_t, int32_t);
uint64_t THIS this_wide(ThisState *, uint64_t);
void *THIS this_pointer(ThisState *, void *);
void THIS this_void(ThisState *, int32_t);
int32_t THIS this_payload(ThisState *, ThisPayload, int32_t);
typedef int32_t (THIS *ThisPayloadEntry)(ThisState *, ThisPayload, int32_t);
int32_t this_payload_apply(ThisPayloadEntry, ThisState *, ThisPayload, int32_t);
ThisLarge THIS this_large(ThisState *, int32_t);
typedef double (THIS *ThisEntry)(ThisState *, int8_t, uint16_t, float, double, uint64_t, int32_t);
typedef void (THIS *ThisNotify)(ThisState *);
typedef int32_t (THIS *ThisIntEntry)(ThisState *, int32_t);
void this_notify_apply(ThisNotify, ThisState *);
int32_t this_int_apply(ThisIntEntry, ThisState *, int32_t);
ThisEntry this_factory(void);
double this_apply(ThisEntry, ThisState *, int8_t, uint16_t, float, double, uint64_t, int32_t);
double THIS this_forward(ThisState *, ThisEntry, int8_t, uint16_t, float, double, uint64_t, int32_t);
#endif
