#include "gate.h"
#include <stdatomic.h>
#include <unistd.h>
static atomic_int ready, released;
int32_t fixture_wait(void) {
    atomic_store(&ready, 1);
    while (!atomic_load(&released)) usleep(1000);
    return 42;
}
int32_t fixture_gate(int32_t mode, int32_t unused) {
    (void)unused;
    if (mode == 1) atomic_store(&released, 1);
    if (mode == 2) { atomic_store(&ready, 0); atomic_store(&released, 0); }
    return atomic_load(&ready);
}
static atomic_int value_destroyed;
static atomic_uintptr_t value_observer;
void fixture_value_record(void) {
    int32_t count = atomic_fetch_add(&value_destroyed, 1) + 1;
    uintptr_t observer = atomic_load(&value_observer);
    if (observer) ((void (*)(int32_t))observer)(count);
}
int32_t fixture_value_count(int32_t reset, int32_t unused) {
    (void)unused;
    if (reset) atomic_store(&value_destroyed, 0);
    return atomic_load(&value_destroyed);
}
int32_t fixture_value_observer(int32_t low, int32_t high) {
    atomic_store(&value_observer, (uint64_t)(uint32_t)low | ((uint64_t)(uint32_t)high << 32));
    return 0;
}
