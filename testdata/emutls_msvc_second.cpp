extern "C" void record_event(int);
struct Second {
    int value;
    Second() : value(9) { record_event(102); }
    ~Second() { record_event(value); }
};
thread_local Second second;
extern "C" int tls_other(void) { return second.value; }
