#include <stdint.h>
extern void record_event(int);
#if defined(__APPLE__)
extern int _tlv_atexit(void (*)(void *), void *, void *);
#define REGISTER_THREAD_EXIT _tlv_atexit
#else
extern int __cxa_thread_atexit(void (*)(void *), void *, void *);
#define REGISTER_THREAD_EXIT __cxa_thread_atexit
#endif
static void event(void *value) { record_event((int)(uintptr_t)value); }
int tls_register(int a, int b) {
    return REGISTER_THREAD_EXIT(event, (void *)(uintptr_t)(a + b), 0);
}
int tls_invalid(int a, int b) { (void)a; (void)b; return REGISTER_THREAD_EXIT(0, 0, 0); }
int tls_budget(int a, int b) {
    (void)a; (void)b;
    int count = 0;
    while (count < 5000 && !REGISTER_THREAD_EXIT(event, (void *)1, 0)) ++count;
    return count;
}
