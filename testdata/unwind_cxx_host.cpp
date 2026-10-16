#include <unwind.h>

#if !defined(__APPLE__)
struct dwarf_eh_bases { void *text, *data, *function; };
extern "C" const void *_Unwind_Find_FDE(const void *, dwarf_eh_bases *);
#endif
static void *saved;
static int events;
extern "C" void cxx_event(int value) { events += value; }
extern "C" void cxx_save(void *pc) { saved = pc; }
extern "C" int cxx_events(int reset, int) {
    int result = events;
    if (reset) events = 0;
    return result;
}
extern "C" int cxx_has_frame(int, int) {
    dwarf_eh_bases bases;
    return saved && _Unwind_Find_FDE(saved, &bases) != nullptr;
}
extern "C" int cxx_host_throw(int a, int b) {
    if (a >= 0) throw a + b;
    throw double(a + b);
}
