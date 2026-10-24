#include <stdint.h>
#ifdef __cplusplus
#define TLS thread_local
extern "C" {
#else
#define TLS _Thread_local
#endif
TLS int tls_shared = 7;
extern TLS uint8_t tls_ld_b, tls_ld_zb;
extern TLS uint16_t tls_ld_h, tls_ld_zh;
extern TLS uint32_t tls_ld_w, tls_ld_zw;
extern TLS uint64_t tls_ld_d, tls_ld_zd;
struct Pair { uint64_t a, b; };
extern TLS struct Pair tls_ld_q, tls_ld_zq;
extern TLS float tls_ld_f;
extern TLS double tls_ld_g;
int tls_ld_check(void) {
    if (tls_ld_b != 0x12 || tls_ld_h != 0x1234 || tls_ld_w != 0x12345678 ||
        tls_ld_d != UINT64_C(0x123456789abcdef0) || tls_ld_q.a != 20 || tls_ld_q.b != 22 ||
        tls_ld_f != 1 || tls_ld_g != 1 || tls_ld_zb != 1 || tls_ld_zh != 1 ||
        tls_ld_zw != 1 || tls_ld_zd != 1 || tls_ld_zq.a != 1 || tls_ld_zq.b != 1) return 0;
    tls_ld_zb = 0; tls_ld_zh = 0; tls_ld_zw = 0; tls_ld_zd = 0;
    tls_ld_zq.a = 0; tls_ld_zq.b = 0;
    return 1;
}
#ifndef __cplusplus
int tls_read(void) { return tls_shared; }
#else
}
#endif
