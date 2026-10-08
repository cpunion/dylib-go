extern void record_event(int);
typedef void (*hook)(void);
static int ready;

static void preinit(void) { record_event(0); }
__attribute__((used, section(".preinit_array")))
static hook const before_all = preinit;

__attribute__((constructor(101)))
static void early(void) { ready = 40; record_event(1); }
static void middle(void) { record_event(2); }
__attribute__((used, section(".init_array.00150")))
static hook const modern_init = middle;
__attribute__((constructor(201)))
static void late(void) { ready += 2; record_event(3); }
static void default_first(void) { record_event(4); }
static void default_second(void) { record_event(5); }
__attribute__((used, section(".ctors")))
static hook const constructors[] = {
    (hook)-1, default_second, 0, default_first, 0
};

static void stop_first(void) { record_event(6); }
static void stop_second(void) { record_event(7); }
__attribute__((used, section(".dtors")))
static hook const destructors[] = {
    (hook)-1, stop_first, 0, stop_second, 0,
#ifdef BAD_LEGACY_FINALIZER
    (hook)1,
#endif
};
__attribute__((destructor(201)))
static void stop_late(void) { record_event(8); }
static void stop_middle(void) { record_event(9); }
__attribute__((used, section(".fini_array.00150")))
static hook const modern_fini = stop_middle;
__attribute__((destructor(101)))
static void stop_early(void) { record_event(10); }

int legacy_root(int a, int b) { return ready+a+b; }
