extern int tls_step(int, int);
int native_tls_forward(int a, int b) { return tls_step(a, b); }
