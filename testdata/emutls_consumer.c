extern _Thread_local int tls_shared;
extern int tls_read(void);
extern int tls_check_alignment(const void *);
static _Thread_local int zero;
static _Thread_local _Alignas(64) int aligned[16];

int tls_step(int a, int b) {
    // A separate translation unit cannot fold this check using the variable's
    // declared alignment: it must inspect the address returned by the helper.
    if (!tls_check_alignment(aligned) || tls_check_alignment((const char *)aligned + 1)) return -1;
    if (tls_read() != tls_shared || zero != tls_shared - 7 || aligned[0] != zero * 2) return -2;
    ++zero;
    aligned[0] += 2;
    return tls_shared++ + a + b;
}
