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
