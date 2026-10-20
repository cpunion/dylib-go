extern "C" void record_event(int);
extern "C" int tls_other(void);
struct First {
    int value;
    First() : value(7) { record_event(101); }
    ~First() { record_event(value); }
};
thread_local First first;
extern "C" int tls_step(int a, int b) {
    if (tls_other() != 9) return -1;
    return a + b + first.value++;
}
