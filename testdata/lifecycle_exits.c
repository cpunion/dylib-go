extern void record_event(int);
extern int atexit(void (*)(void));
extern int __cxa_atexit(void (*)(void *), void *, void *);
extern void __cxa_finalize(void *);
extern void *__dso_handle;
static int tag1, tag2;
static int eleven = 11, twelve = 12, fourteen = 14;
static void tagged(void *value) { record_event(*(int *)value); }
static void plain(void) { record_event(13); }
static void recursive(void *value) {
    record_event(*(int *)value);
    __cxa_atexit(tagged, &fourteen, &tag2);
    __cxa_finalize((void *)0);
}
int register_selective(int a, int b) {
    int err = __cxa_atexit(tagged, &eleven, &tag1);
    err |= __cxa_atexit(recursive, &twelve, &tag2);
    err |= atexit(plain);
    return err ? -1 : a+b;
}
int finalize_tag(int a, int b) { __cxa_finalize(&tag1); return a+b; }
int finalize_all(int a, int b) { __cxa_finalize((void *)0); return a+b; }
int dso_self(int a, int b) { return __dso_handle == (void *)&__dso_handle ? a+b : -1; }
