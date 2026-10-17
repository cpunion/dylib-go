#include <stdint.h>
extern _Thread_local int tls_shared;
extern int tls_read(void);
static _Thread_local int zero;
static _Thread_local _Alignas(64) int aligned[16];

int tls_step(int a, int b) {
    if ((uintptr_t)aligned % 64) return -1;
    if (tls_read() != tls_shared || zero != tls_shared - 7 || aligned[0] != zero * 2) return -2;
    ++zero;
    aligned[0] += 2;
    return tls_shared++ + a + b;
}
