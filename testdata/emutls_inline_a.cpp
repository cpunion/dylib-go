#include "emutls_inline.h"
extern "C" int tls_inline_a(int a, int b) { return ++tls_inline_value + a + b; }
