extern void optional_event(int, int) __attribute__((weak));
int weak_call(int a, int b) {
    optional_event(a, b);
    return a + b;
}
