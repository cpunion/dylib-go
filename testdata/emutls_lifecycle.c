extern void record_event(int);
extern int atexit(void (*)(void));
static _Thread_local int value = 7;
static void exit_hook(void) { record_event(value++); }
static void start(void) { record_event(value++); atexit(exit_hook); }
static void stop(void) { record_event(value++); }
#if defined(_WIN32)
__attribute__((used, section(".CRT$XCU"))) static void (*const init)(void) = start;
__attribute__((used, section(".CRT$XTU"))) static void (*const fini)(void) = stop;
#elif defined(__APPLE__)
__attribute__((used, section("__DATA,__mod_init_func,mod_init_funcs"))) static void (*const init)(void) = start;
__attribute__((used, section("__DATA,__mod_term_func,mod_term_funcs"))) static void (*const fini)(void) = stop;
#else
__attribute__((used, section(".init_array"))) static void (*const init)(void) = start;
__attribute__((used, section(".fini_array"))) static void (*const fini)(void) = stop;
#endif
int tls_lifecycle(int a, int b) { return value++ + a + b; }
