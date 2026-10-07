extern void record_event(int);
static void first(void) { record_event(3); }
static void second(void) { record_event(4); }
static void stop_first(void) { record_event(6); }
static void stop_second(void) { record_event(5); }
#if defined(_WIN32)
__attribute__((used, section(".CRT$XCT"))) static void (*const init1)(void) = first;
__attribute__((used, section(".CRT$XCV"))) static void (*const init2)(void) = second;
__attribute__((used, section(".CRT$XPB"))) static void (*const term1)(void) = stop_second;
__attribute__((used, section(".CRT$XTB"))) static void (*const term2)(void) = stop_first;
#elif defined(__APPLE__)
__attribute__((used, section("__DATA,__mod_init_func,mod_init_funcs"))) static void (*const init1)(void) = first;
__attribute__((used, section("__DATA,__mod_init_func,mod_init_funcs"))) static void (*const init2)(void) = second;
__attribute__((used, section("__DATA,__mod_term_func,mod_term_funcs"))) static void (*const term1)(void) = stop_first;
__attribute__((used, section("__DATA,__mod_term_func,mod_term_funcs"))) static void (*const term2)(void) = stop_second;
#else
static void preinit(void) { record_event(0); }
__attribute__((used, section(".preinit_array"))) static void (*const preinit1)(void) = preinit;
__attribute__((used, section(".init_array.00100"))) static void (*const init1)(void) = first;
__attribute__((used, section(".init_array.00200"))) static void (*const init2)(void) = second;
__attribute__((used, section(".fini_array.00100"))) static void (*const term1)(void) = stop_first;
__attribute__((used, section(".fini_array.00200"))) static void (*const term2)(void) = stop_second;
#endif
int array_root(int a, int b) { return a+b; }
