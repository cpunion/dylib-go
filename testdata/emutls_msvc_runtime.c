#include <stdint.h>
extern void tls_runtime_init(void);
extern int tls_runtime_plain(void (*)(void));
extern int tls_runtime_argument(void (*)(void *), void *, void *);
static int events;
void tls_constructor(void) { tls_runtime_init(); events = events * 10 + 4; }
static void first(void) { events = events * 10 + 1; }
static void second(void) { events = events * 10 + 2; }
static void argument(void *value) { events = events * 10 + (int)(uintptr_t)value; }
int tls_use(int a, int b) {
    (void)a; (void)b;
    tls_runtime_init();
    tls_runtime_init();
    if (tls_runtime_plain(first) || tls_runtime_argument(argument, (void *)3, 0) || tls_runtime_plain(second)) return -1;
    return events;
}
int tls_events(int a, int b) { (void)a; (void)b; return events; }
