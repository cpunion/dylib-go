int datum = 42;
int add(int a, int b) { return a + b; }
double mixed(float a, double b) { return a + b; }
_Bool truth(_Bool value) { return !value; }
void *pointer(void *value) { return value; }
int variable(int first, ...) {
    __builtin_va_list ap;
    __builtin_va_start(ap, first);
    int second = __builtin_va_arg(ap, int);
    double third = __builtin_va_arg(ap, double);
    __builtin_va_end(ap);
    return first + second + (int)third;
}
