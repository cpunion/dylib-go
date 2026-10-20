extern int __cxa_atexit(void (*)(void *), void *, void *);
extern int atexit(void (*)(void));
extern void record_event(int);
static int first = 11, second = 22;
#if defined(_WIN32) && defined(__i386__)
#define THIS __attribute__((thiscall))
#else
#define THIS
#endif
static void THIS object_exit(void *value) { record_event(value == &first ? *(int *)value : -11); }
static void argument_exit(void *value) { record_event(value == &second ? *(int *)value : -22); }
static void plain_exit(void) { record_event(33); }
__attribute__((constructor)) static void initialize(void) {
    __cxa_atexit((void (*)(void *))object_exit, &first, 0);
    __cxa_atexit(argument_exit, &second, 0);
    atexit(plain_exit);
}
int mixed_ready(int a, int b) { return a + b; }
