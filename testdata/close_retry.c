extern void record_event(int);
extern int retained_add(int, int);
extern int atexit(void (*)(void));
static void exit_hook(void) { record_event(retained_add(20, 22)); }
static void finalizer(void) { record_event(retained_add(1, 2)); }
#if defined(_WIN32)
__attribute__((used, section(".CRT$XTU"))) static void (*const fini)(void) = finalizer;
#elif defined(__APPLE__)
__attribute__((used, section("__DATA,__mod_term_func,mod_term_funcs"))) static void (*const fini)(void) = finalizer;
#else
__attribute__((used, section(".fini_array"))) static void (*const fini)(void) = finalizer;
#endif
int close_retry_register(int a, int b) {
    return atexit(exit_hook) ? -1 : retained_add(a, b);
}
