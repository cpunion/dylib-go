extern "C" void compact_target(int);
struct CompactGuard { ~CompactGuard() noexcept; };
extern "C" int compact_catch(int value) {
#if defined(DYLIB_CXX_LARGE_STACK)
    volatile unsigned char stack[8192];
    stack[0] = 1;
    stack[8191] = 2;
    asm volatile("" : : "r"(&stack[0]) : "memory");
#endif
    CompactGuard guard;
    try { compact_target(value); }
    catch (int caught) { return caught + 22; }
    return value;
}
extern "C" int compact_leaf(int a, int b) { return a + b; }
