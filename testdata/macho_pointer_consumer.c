extern int stub_eval(int, int);
extern int (*lazy_eval)(int, int);
extern int (*nonlazy_eval)(int, int);
extern int *local_pointer;

int consume_pointers(int a, int b) {
    return stub_eval(a, b) + lazy_eval(a, b) + nonlazy_eval(a, b) + *local_pointer;
}
