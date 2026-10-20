extern void record_event(int);
extern int tls_step(int, int);
static int initialize(void) {
    record_event(100);
    return tls_step(20, 22) == 49 ? 7 : 99;
}
#if defined(_WIN32)
#define INIT_TABLE ".CRT$XIC"
#elif defined(__APPLE__)
#define INIT_TABLE "__DATA,__c_init"
#else
#define INIT_TABLE ".dylib_c_init"
#endif
// POSIX tests use the existing private C initializer table adapter; only COFF
// supplies this int-returning initialization format in production.
__attribute__((used, section(INIT_TABLE))) static int (*const table)(void) = initialize;
