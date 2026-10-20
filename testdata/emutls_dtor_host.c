#include "emutls_threads.c"
static atomic_int event_count, event_sum, gate, entered;
static int events[128];
EXPORT void record_event(int value) {
    int index = atomic_fetch_add_explicit(&event_count, 1, memory_order_acq_rel);
    if (index < 128) events[index] = value;
    atomic_fetch_add(&event_sum, value);
    atomic_fetch_add_explicit(&entered, 1, memory_order_release);
    while (atomic_load_explicit(&gate, memory_order_acquire)) {
#if defined(_WIN32)
        Sleep(1);
#else
        struct timespec delay = {0, 1000000};
        nanosleep(&delay, 0);
#endif
    }
}
EXPORT int recorded_event(int index, int unused) {
    (void)unused;
    if (index < 0) return atomic_load(&event_count);
    return index < atomic_load(&event_count) && index < 128 ? events[index] : -1;
}
EXPORT int tls_dtor_sum(int a, int b) { (void)a; (void)b; return atomic_load(&event_sum); }
EXPORT int tls_dtor_gate(int enabled, int unused) { (void)unused; atomic_store(&gate, enabled); return 0; }
EXPORT int tls_dtor_entered(int a, int b) { (void)a; (void)b; return atomic_load_explicit(&entered, memory_order_acquire); }
