static _Thread_local int seed = 2;

int cli_tls_add(int a, int b) {
    seed += a + b;
    return seed;
}

struct cli_tls_pair { int a, b; };
struct cli_tls_pair cli_tls_pair(struct cli_tls_pair value) {
    value.a += seed;
    value.b -= seed;
    return value;
}
