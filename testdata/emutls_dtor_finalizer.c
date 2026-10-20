extern void record_event(int);
#if defined(DYLIB_TLS_LATE)
extern void tls_register_late(void);
#endif
__attribute__((destructor)) static void finish(void) {
    record_event(900);
#if defined(DYLIB_TLS_LATE)
    tls_register_late();
#endif
}
