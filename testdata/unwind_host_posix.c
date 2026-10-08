#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdint.h>
#include <unwind.h>

#if !defined(__APPLE__)
struct dwarf_eh_bases { void *tbase, *dbase, *func; };
extern const void *_Unwind_Find_FDE(const void *, struct dwarf_eh_bases *);
#endif
static uintptr_t saved_pc;
static int frame_count;
static _Unwind_Reason_Code capture(struct _Unwind_Context *context, void *argument) {
    (void)argument;
    uintptr_t pc = _Unwind_GetIP(context);
    Dl_info info;
    if (pc && !dladdr((void *)(pc-1), &info)) {
        saved_pc = pc-1;
        if (++frame_count == 2) return _URC_END_OF_STACK;
    }
    return _URC_NO_REASON;
}
int capture_raw_frames(int a, int b) {
    frame_count = 0;
    _Unwind_Backtrace(capture, 0);
    return frame_count == 2 ? a+b : -frame_count-1;
}
int has_saved_frame(int a, int b) {
    (void)a; (void)b;
    struct dwarf_eh_bases bases;
    return saved_pc && _Unwind_Find_FDE((const void *)saved_pc, &bases) != 0;
}
