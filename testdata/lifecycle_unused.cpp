extern "C" void record_event(int);
struct Unused {
    Unused() { record_event(999); }
    ~Unused() { record_event(999); }
};
static Unused unused;
extern "C" int unused_root(int a, int b) { return a+b; }
