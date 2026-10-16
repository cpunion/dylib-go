// Force real DWARF on Apple targets even when compact unwind can describe the
// frame. The CFI no-op does not change native instructions or stack layout.
#if defined(__APPLE__) && !defined(DYLIB_CXX_COMPACT)
#define DYLIB_CXX_DWARF() asm volatile(".cfi_escape 0x00")
#else
#define DYLIB_CXX_DWARF() ((void)0)
#endif
