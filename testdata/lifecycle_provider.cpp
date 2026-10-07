extern "C" void record_event(int);
static int ready;
struct Provider {
    Provider() { ready = 40; record_event(1); }
    ~Provider() { record_event(9); }
};
static Provider provider;
extern "C" int lifecycle_dependency(void) { return ready; }
