#include <stdint.h>
#if defined(_WIN32)
#define EXPORT __declspec(dllexport)
#else
#define EXPORT
#endif
struct pair { int32_t a,b; };
struct nested { int8_t tag; struct pair p; double extra; };
struct big { int64_t a,b,c; };
struct floats { float x,y; };
EXPORT int32_t invoke_i32(int32_t (*fn)(int32_t,int32_t), int32_t a, int32_t b) { return fn(a,b); }
EXPORT int32_t invoke_zero(int32_t (*fn)(void)) { return fn(); }
EXPORT double invoke_mixed(double (*fn)(int32_t,double,float,uint64_t)) { return fn(10,20.5,1.5,10); }
EXPORT int32_t invoke_narrow(int8_t (*fn)(int8_t), int32_t value) { return fn((int8_t)value); }
EXPORT uint32_t invoke_u8(uint8_t (*fn)(uint8_t), uint32_t value) { return fn((uint8_t)value); }
EXPORT int32_t invoke_i16(int16_t (*fn)(int16_t), int32_t value) { return fn((int16_t)value); }
EXPORT uint32_t invoke_u16(uint16_t (*fn)(uint16_t), uint32_t value) { return fn((uint16_t)value); }
EXPORT int64_t invoke_i64(int64_t (*fn)(int64_t), int64_t value) { return fn(value); }
EXPORT uint64_t invoke_u64(uint64_t (*fn)(uint64_t), uint64_t value) { return fn(value); }
EXPORT uint32_t invoke_bool(_Bool (*fn)(_Bool), uint32_t value) { return fn(value != 0); }
EXPORT float invoke_float(float (*fn)(float), float value) { return fn(value); }
EXPORT void *invoke_pointer(void *(*fn)(void *), void *value) { return fn(value); }
EXPORT int32_t invoke_void(void (*fn)(int32_t), int32_t value) { fn(value); return value; }
EXPORT struct pair invoke_pair(struct pair (*fn)(struct pair), struct pair value) { return fn(value); }
EXPORT struct nested invoke_nested(struct nested (*fn)(struct nested), struct nested value) { return fn(value); }
EXPORT struct big invoke_big(struct big (*fn)(struct big), struct big value) { return fn(value); }
EXPORT struct floats invoke_floats(struct floats (*fn)(struct floats), struct floats value) { return fn(value); }
static int32_t (*stored)(int32_t,int32_t);
EXPORT void store_callback(int32_t (*fn)(int32_t,int32_t)) { stored=fn; }
EXPORT int32_t call_stored(int32_t a, int32_t b) { return stored ? stored(a,b) : -1; }
EXPORT void clear_callback(void) { stored=0; }
#if defined(_WIN32) && (defined(__i386__) || defined(_M_IX86))
EXPORT int32_t invoke_stdcall(int32_t (__stdcall *fn)(int32_t,int32_t), int32_t a, int32_t b) { return fn(a,b); }
EXPORT int32_t invoke_fastcall(int32_t (__fastcall *fn)(int32_t,int32_t), int32_t a, int32_t b) { return fn(a,b); }
#endif
