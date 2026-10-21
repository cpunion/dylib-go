extern void missing_native_thread_exit_dependency(void);
#ifdef __APPLE__
void _tlv_atexit(void) { missing_native_thread_exit_dependency(); }
#else
void __cxa_thread_atexit(void) { missing_native_thread_exit_dependency(); }
#endif
