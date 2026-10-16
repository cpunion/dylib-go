#include "unwind_cxx_dwarf.h"

extern "C" int cxx_throw(int, int);
extern "C" int cxx_host_throw(int, int);
extern "C" void cxx_event(int);
extern "C" void cxx_save(void *);
struct OuterGuard { ~OuterGuard() { cxx_event(10); } };
static __attribute__((noinline)) int relay(int a, int b) {
    DYLIB_CXX_DWARF();
#if defined(DYLIB_CXX_LARGE_STACK)
    volatile unsigned char stack[8192];
    stack[0] = 1;
    stack[8191] = 2;
    asm volatile("" : : "r"(&stack[0]) : "memory");
#endif
    try { return cxx_throw(a, b); }
    catch (int value) { cxx_event(100); throw; }
}
extern "C" int cxx_catch(int a, int b) {
    DYLIB_CXX_DWARF();
    // Query an instruction inside the frame, avoiding entry/return boundaries.
    cxx_save(reinterpret_cast<char *>(&cxx_catch) + 4);
    OuterGuard guard;
    try {
        if (a == 99) return cxx_host_throw(20, 22);
        return relay(a, b);
    } catch (int value) { return value; }
      catch (double value) { return int(value); }
}
extern "C" int cxx_plain(int a, int b) { return a + b; }
