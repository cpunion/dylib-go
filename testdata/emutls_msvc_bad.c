__attribute__((used, section(".CRT$XDU")))
static void (*const table)(void) = (void (*)(void))1;
int tls_bad_unused(int a, int b) { return a + b; }
