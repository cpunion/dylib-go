#include <windows.h>
#include <stdint.h>

__declspec(dllexport) int capture_raw_frames(int a, int b) {
    CONTEXT context;
    RtlCaptureContext(&context);
    int count = 0;
    for (int i = 0; i < 64; ++i) {
#if defined(__aarch64__)
        DWORD64 pc = context.Pc;
#else
        DWORD64 pc = context.Rip;
#endif
        if (!pc) break;
        DWORD64 base = 0;
        PRUNTIME_FUNCTION entry = RtlLookupFunctionEntry(pc, &base, NULL);
        if (!entry) break; // Stop at leaf/foreign frames; never walk a Go stack.
        HMODULE module;
        if (!GetModuleHandleExW(GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS |
                               GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT,
                               (LPCWSTR)(uintptr_t)pc, &module)) {
            ++count;
            if (count == 2) return a+b;
        }
        PVOID handler_data = NULL;
        DWORD64 frame = 0;
        RtlVirtualUnwind(UNW_FLAG_NHANDLER, base, pc, entry, &context,
                         &handler_data, &frame, NULL);
    }
    return -count-1;
}
