extern void record_event(int);
extern int __tlregdtor(void (*)(void));
static int stored;
static void event(void) { record_event(stored); }
int tls_register(int a, int b) { stored = a + b; return __tlregdtor(event); }
int tls_invalid(int a, int b) { (void)a; (void)b; return __tlregdtor(0); }
int tls_budget(int a, int b) {
    (void)a; (void)b;
    stored = 1;
    int count = 0;
    while (count < 5000 && !__tlregdtor(event)) ++count;
    return count;
}
