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

#if defined(__APPLE__)
#include <stdint.h>
using Finder = int (*)(uintptr_t, void *);
extern "C" int __unw_add_find_dynamic_unwind_sections(Finder) __attribute__((weak_import));
extern "C" int __unw_remove_find_dynamic_unwind_sections(Finder) __attribute__((weak_import));
static volatile int finder_salts[8];
#define FINDER(N) static int empty_finder_##N(uintptr_t, void *) { (void)finder_salts[N]; return 0; }
FINDER(0) FINDER(1) FINDER(2) FINDER(3) FINDER(4) FINDER(5) FINDER(6) FINDER(7)
static Finder empty_finders[] = {empty_finder_0, empty_finder_1, empty_finder_2, empty_finder_3,
                                 empty_finder_4, empty_finder_5, empty_finder_6, empty_finder_7};
static int finder_count;
// Exhaust the real native finder slots to exercise failed-link ownership and
// retry. Every installed callback belongs to this observer's retained library.
extern "C" int cxx_unwind_slots(int fill, int) {
    if (!__unw_add_find_dynamic_unwind_sections || !__unw_remove_find_dynamic_unwind_sections) return -1;
    if (fill) {
        while (finder_count < 8 && __unw_add_find_dynamic_unwind_sections(empty_finders[finder_count]) == 0) ++finder_count;
        return finder_count;
    }
    int previous = finder_count;
    while (finder_count) {
        int status = __unw_remove_find_dynamic_unwind_sections(empty_finders[finder_count-1]);
        if (status) return status;
        --finder_count;
    }
    return previous;
}
#endif
