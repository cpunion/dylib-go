_Thread_local int tls_shared = 7;
int tls_read(void) { return tls_shared; }
