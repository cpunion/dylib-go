extern int capture_raw_frames(int, int);
__attribute__((noinline))
static int inner(int a, int b) {
    volatile int values[8];
    values[0] = a;
    values[7] = b;
    return capture_raw_frames(values[0], values[7]);
}
int unwind_root(int a, int b) {
    volatile int result = inner(a, b);
    return result;
}
