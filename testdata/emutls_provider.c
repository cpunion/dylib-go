#include <stdint.h>
_Thread_local int tls_shared = 7;
int tls_read(void) { return tls_shared; }
int tls_check_alignment(const void *p) { return (uintptr_t)p % 64 == 0; }
