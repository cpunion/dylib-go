#include "unwind_host_windows.c"

// Unwind one controlled raw frame at seven instruction boundaries. In
// particular, a resumed parent must apply its full original unwind program.
__declspec(dllexport) int verify_chain_context(uintptr_t pc, int stage) {
    if (stage < 0 || stage >= 7) return -1;
    __declspec(align(16)) DWORD64 stack[16] = {0};
    stack[5] = 0x3333;
    stack[6] = 0x4444;
    stack[7] = 0x12345678;
    CONTEXT context = {0};
    context.ContextFlags = CONTEXT_FULL;
    context.Rip = pc;
    context.Rsp = (DWORD64)(uintptr_t)(stack + (stage == 0 ? 7 : 0));
    context.Rbx = stage == 6 ? 0x3333 : 0x1111;
    context.Rsi = stage >= 5 ? 0x4444 : 0x2222;
    DWORD64 base = 0, frame = 0;
    PRUNTIME_FUNCTION entry = RtlLookupFunctionEntry(pc, &base, NULL);
    if (!entry) return -2;
    PVOID handler_data = NULL;
    RtlVirtualUnwind(UNW_FLAG_NHANDLER, base, pc, entry, &context,
                     &handler_data, &frame, NULL);
    int mismatch = 0;
    if (context.Rip != stack[7]) mismatch |= 1;
    if (context.Rsp != (DWORD64)(uintptr_t)(stack + 8)) mismatch |= 2;
    if (context.Rbx != (DWORD64)(stage >= 3 ? 0x3333 : 0x1111)) mismatch |= 4;
    if (context.Rsi != (DWORD64)(stage >= 4 ? 0x4444 : 0x2222)) mismatch |= 8;
    return mismatch ? -(100 + stage*16 + mismatch) : 42;
}
