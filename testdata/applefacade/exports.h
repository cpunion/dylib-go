#ifndef DYLIB_APPLE_FACADE_H
#define DYLIB_APPLE_FACADE_H
#include <stdint.h>
#ifdef __cplusplus
extern "C" {
#endif

// Handles own one reference. Retain adds one; release consumes one. Calls are
// synchronous and serialized. The observer remains borrowed until final release;
// final destruction calls it with -1. Native exceptions never cross this API.
typedef int32_t (*AppleObserver)(int32_t);
void *apple_create(int32_t initial, AppleObserver observer);
void *apple_retain(void *handle);
void apple_release(void *handle);
int32_t apple_add(void *handle, int32_t delta);
int32_t apple_live(void);

#ifdef __cplusplus
}
#endif
#endif
