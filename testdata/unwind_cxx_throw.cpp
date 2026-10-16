extern "C" void cxx_event(int);
struct InnerGuard { ~InnerGuard() { cxx_event(1); } };
extern "C" __attribute__((noinline)) int cxx_throw(int a, int b) {
    InnerGuard guard;
    if (a >= 0) throw a + b;
    throw double(a + b);
}
