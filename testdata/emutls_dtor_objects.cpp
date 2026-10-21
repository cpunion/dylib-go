extern "C" void record_event(int);
extern "C" void *__dso_handle;
extern "C" int atexit(void (*)(void));
#if defined(__APPLE__)
extern "C" int _tlv_atexit(void (*)(void *), void *, void *);
#define REGISTER_THREAD_EXIT _tlv_atexit
#else
extern "C" int __cxa_thread_atexit(void (*)(void *), void *, void *);
#define REGISTER_THREAD_EXIT __cxa_thread_atexit
#endif

thread_local int marker = 3;
#if defined(DYLIB_TLS_DYNAMIC_INIT)
extern "C" int tls_guard_probe(int, int) { return marker; }
#endif
static void nested(void *value) {
    if (value != &marker) record_event(-1);
    else record_event(700 + marker);
}
struct TLSObject {
    int value;
#if defined(DYLIB_TLS_DYNAMIC_INIT)
    explicit TLSObject(int initial) : value(initial) { record_event(1000 + initial); }
#endif
    ~TLSObject() {
        record_event(value);
        if (REGISTER_THREAD_EXIT(nested, &marker, &__dso_handle)) record_event(-2);
    }
};
thread_local TLSObject first{7}, second{9};
extern "C" int tls_step(int a, int b) {
#if defined(DYLIB_TLS_WORK_CALLBACK)
    if (marker == 3) record_event(0);
#endif
    ++marker;
    if (second.value != 9) return -100;
    return a + b + first.value++;
}

static void last_thread_exit(void *) { record_event(903); }
static void late_exit() {
    record_event(902);
    REGISTER_THREAD_EXIT(last_thread_exit, nullptr, &__dso_handle);
}
static void late_thread_exit(void *) {
    record_event(901);
    atexit(late_exit);
}
extern "C" void tls_register_late() {
    REGISTER_THREAD_EXIT(late_thread_exit, nullptr, &__dso_handle);
}
