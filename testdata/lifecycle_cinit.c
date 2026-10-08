extern void record_event(int);
extern int atexit(void (*)(void));
typedef void (*hook)(void);
typedef int (*c_hook)(void);
#ifndef FAIL_CODE
#define FAIL_CODE 0
#endif
static int ready;
static void exit_first(void) { record_event(8); }
static void exit_second(void) { record_event(7); }
static int first(void) {
    ready = 40;
    record_event(1);
    return atexit(exit_first);
}
static int second(void) {
    ready += 2;
    record_event(2);
    if (atexit(exit_second)) return 99;
    return FAIL_CODE;
}
static int third(void) { record_event(3); return 0; }
static void construct(void) { record_event(4); }
static void preterminate(void) { record_event(5); }
static void terminate(void) { record_event(6); }

#if defined(_WIN32)
// Intentionally emit XC and XIC before XIB; execution sorts table names/phases.
__attribute__((used, section(".CRT$XCC"))) static hook const cpp_init = construct;
__attribute__((used, section(".CRT$XIC"))) static c_hook const c_init_later[] = {
    0, second, third,
#ifdef BAD_CINIT_POINTER
    (c_hook)1,
#endif
};
__attribute__((used, section(".CRT$XIB"))) static c_hook const c_init_first = first;
__attribute__((used, section(".CRT$XIA"))) static c_hook const c_begin = 0;
__attribute__((used, section(".CRT$XIZ"))) static c_hook const c_end = 0;
__attribute__((used, section(".CRT$XPB"))) static hook const cpp_preterm = preterminate;
__attribute__((used, section(".CRT$XTB"))) static hook const cpp_term = terminate;
#else
// POSIX fixtures exercise the shared invocation/lifetime machinery by marking
// this private test table in Go. Production ELF/Mach-O parsing does not assign
// the COFF int-returning contract to ordinary initialization arrays.
#if defined(__APPLE__)
#define C_INIT "__DATA,__c_init"
#define CPP_INIT "__DATA,__mod_init_func,mod_init_funcs"
#define CPP_FINI "__DATA,__mod_term_func,mod_term_funcs"
#else
#define C_INIT ".dylib_c_init"
#define CPP_INIT ".init_array"
#define CPP_FINI ".fini_array"
#endif
__attribute__((used, section(C_INIT))) static c_hook const c_init[] = {
    first, 0, second, third,
#ifdef BAD_CINIT_POINTER
    (c_hook)1,
#endif
};
__attribute__((used, section(CPP_INIT))) static hook const cpp_init = construct;
__attribute__((used, section(CPP_FINI))) static hook const cpp_fini[] = {
    terminate, preterminate
};
#endif
int cinit_root(int a, int b) {
#if defined(FORCE_DWARF) && defined(__APPLE__)
    __asm__ volatile(".cfi_escape 0x00");
#endif
    return ready+a+b;
}
