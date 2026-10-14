extern void record_event(int);
static int ready;
void startup_init_marker(void){ ready=42; record_event(2); }
void startup_fini_marker(void){ record_event(5); }
#ifndef NO_ARRAYS
static void before(void){ record_event(1); }
static void after(void){ record_event(3); }
static void finish(void){ record_event(4); }
__attribute__((used,section(".preinit_array"))) static void (*preinit)(void)=before;
__attribute__((used,section(".init_array"))) static void (*init)(void)=after;
__attribute__((used,section(".fini_array"))) static void (*fini)(void)=finish;
#endif
int startup_root(int a,int b){ return ready+a+b; }
