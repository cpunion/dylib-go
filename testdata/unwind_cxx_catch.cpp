extern "C" int cxx_throw(int, int);
extern "C" int cxx_host_throw(int, int);
extern "C" void cxx_event(int);
extern "C" void cxx_save(void *);
struct OuterGuard { ~OuterGuard() { cxx_event(10); } };
static __attribute__((noinline)) int relay(int a, int b) {
    try { return cxx_throw(a, b); }
    catch (int value) { cxx_event(100); throw; }
}
extern "C" int cxx_catch(int a, int b) {
    cxx_save(reinterpret_cast<void *>(&cxx_catch));
    OuterGuard guard;
    try {
        if (a == 99) return cxx_host_throw(20, 22);
        return relay(a, b);
    } catch (int value) { return value; }
      catch (double value) { return int(value); }
}
extern "C" int cxx_plain(int a, int b) { return a + b; }
