extern unsigned int size_value32, size_plus32, size_minus32;
extern unsigned int size_local32, size_common32, size_weak32;
#ifdef __x86_64__
extern unsigned long long size_value64;
#endif
int eval(int a, int b) {
    if (size_value32 != 37 || size_plus32 != 42 || size_minus32 != 32 ||
        size_local32 != 9 || size_common32 != 29 || size_weak32 != 0)
        return -1;
#ifdef __x86_64__
    if (size_value64 != 42)
        return -2;
#endif
    return a + b;
}
