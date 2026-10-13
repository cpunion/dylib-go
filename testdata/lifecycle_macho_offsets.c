extern void record_event(int);
static int ready;

/* Keep a real initializer at original image offset zero. */
void offset_first(void) { ready = 20; record_event(3); }
static void offset_second(void) { ready += 22; record_event(4); }
static void pointer_first(void) { record_event(2); }
static void stop_first(void) { record_event(6); }
static void stop_second(void) { record_event(5); }

/* Tests apply the linker's pointer-to-offset encoding to this first table. */
__attribute__((used, section("__DATA,__mod_init_func,mod_init_funcs")))
static void (*const offsets[])(void) = {offset_first, offset_second};
__attribute__((used, section("__DATA,__other_inits,mod_init_funcs")))
static void (*const pointers[])(void) = {pointer_first};
__attribute__((used, section("__DATA,__mod_term_func,mod_term_funcs")))
static void (*const stops[])(void) = {stop_first, stop_second};

int offset_root(int a, int b) { return ready + a + b; }
