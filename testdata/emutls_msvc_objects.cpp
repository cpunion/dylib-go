extern "C" void record_event(int);
extern "C" int atexit(void (*)(void));
extern "C" int __tlregdtor(void (*)(void));
extern "C" void __dyn_tls_on_demand_init(void);

thread_local int marker = 3;
static void nested() { record_event(700 + marker); }
struct TLSObject {
    int value;
#if defined(DYLIB_TLS_DYNAMIC_INIT)
    explicit TLSObject(int initial) : value(initial) { record_event(1000 + initial); }
#endif
    ~TLSObject() {
        record_event(value);
        if (__tlregdtor(nested)) record_event(-2);
    }
};
thread_local TLSObject first{7}, second{9};
extern "C" int tls_guard_probe(int a, int b) {
    (void)a; (void)b;
    __dyn_tls_on_demand_init();
    return marker;
}
extern "C" int tls_step(int a, int b) {
#if defined(DYLIB_TLS_WORK_CALLBACK)
    if (marker == 3) record_event(0);
#endif
    ++marker;
    if (second.value != 9) return -100;
    return a + b + first.value++;
}

static void last_thread_exit() { record_event(903); }
static void late_exit() { record_event(902); __tlregdtor(last_thread_exit); }
static void late_thread_exit() { record_event(901); atexit(late_exit); }
extern "C" void tls_register_late() { __tlregdtor(late_thread_exit); }
