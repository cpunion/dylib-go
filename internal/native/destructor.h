#ifndef DYLIB_GO_DESTRUCTOR_H
#define DYLIB_GO_DESTRUCTOR_H

#if defined(_WIN32) && defined(__i386__)
// Clang's GNU Windows i386 producer registers x86_thiscallcc C++ destructors
// directly with __cxa[_thread]_atexit. Manual registrations can be cdecl.
// Put the sole argument in ECX and on the stack for these two entry contracts.
// Keep a 16-byte call boundary and restore our frame independently of cleanup.
// Native exceptions must not escape a destructor.
__attribute__((naked, noinline))
static void dylib_call_destructor(void (*function)(void *), void *argument) {
    __asm__ __volatile__(
        "pushl %ebp\n\t"
        "movl %esp, %ebp\n\t"
        "movl 8(%ebp), %eax\n\t"
        "movl 12(%ebp), %ecx\n\t"
        "andl $-16, %esp\n\t"
        "subl $16, %esp\n\t"
        "movl %ecx, (%esp)\n\t"
        "call *%eax\n\t"
        "movl %ebp, %esp\n\t"
        "popl %ebp\n\t"
        "ret\n\t");
}
#else
static inline void dylib_call_destructor(void (*function)(void *), void *argument) {
    function(argument);
}
#endif

#endif
