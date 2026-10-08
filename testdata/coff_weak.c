#include <stdint.h>
/* Keep the fallback in a separate section so the assembler cannot resolve
   the weak call before the loader has a chance to select a strong override. */
__attribute__((section(".text$fallback")))
int32_t fallback(int32_t a, int32_t b) { return a+b; }
extern int32_t optional(int32_t, int32_t);
#if defined(__i386__) || defined(_M_IX86)
__asm__(".weak _optional\n.set _optional,_fallback");
#else
__asm__(".weak optional\n.set optional,fallback");
#endif
int32_t call_weak(int32_t a, int32_t b) { return optional(a,b); }
