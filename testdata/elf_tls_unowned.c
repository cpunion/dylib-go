extern void missing_native_tls_dependency(void);
#if defined(__i386__)
void ___tls_get_addr(void) { missing_native_tls_dependency(); }
#else
void __tls_get_addr(void) { missing_native_tls_dependency(); }
#endif
