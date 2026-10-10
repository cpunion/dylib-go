#include <stdint.h>
#include <stddef.h>
#ifdef _WIN32
#define API __declspec(dllexport)
#else
#define API
#endif
typedef struct { int8_t tag; double value; _Bool truth; int32_t values[2]; void *next; } Owned;
static Owned *saved;
static int32_t *saved_array;
API void owned_store(Owned *value) { saved = value; }
API void *owned_pointer(void) { return saved; }
API int32_t owned_sum(void) { return saved ? saved->values[0] + saved->values[1] : -1; }
API void owned_mutate(int32_t delta) {
    saved->tag += 1; saved->value += 1.5; saved->truth = !saved->truth;
    saved->values[0] += delta; saved->values[1] += delta;
}
API uint64_t owned_size(void) { return sizeof(Owned); }
API uint64_t owned_alignment(void) { return _Alignof(Owned); }
API uint64_t owned_offset(int32_t index) {
    switch (index) {
    case 0: return offsetof(Owned,tag);
    case 1: return offsetof(Owned,value);
    case 2: return offsetof(Owned,truth);
    case 3: return offsetof(Owned,values);
    case 4: return offsetof(Owned,next);
    default: return UINT64_MAX;
    }
}
API int32_t owned_callback(int32_t (*entry)(void *)) { return entry(saved); }
API void array_store(int32_t *value) { saved_array = value; }
API int32_t array_sum(void) { return saved_array ? saved_array[0] + saved_array[1] : -1; }
API void array_mutate(int32_t delta) { saved_array[0] += delta; saved_array[1] += delta; }
