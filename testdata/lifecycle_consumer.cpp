extern "C" void record_event(int);
extern "C" int lifecycle_dependency(void);
extern "C" int atexit(void (*)(void));
static int ready;
struct Consumer {
    Consumer() { ready = lifecycle_dependency() + 2; record_event(2); }
    ~Consumer() { record_event(8); }
};
static Consumer consumer;
static void later(void) { record_event(7); }
extern "C" int initialized_add(int a, int b) { return ready + a + b; }
extern "C" int register_later(int a, int b) { return atexit(later) + a + b; }
