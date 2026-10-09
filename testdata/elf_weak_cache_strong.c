extern void optional_event(int, int);
int strong_call(int a, int b) {
    optional_event(a, b);
    return a + b;
}
