static int bias = 7;
static int *p = &bias;
int pointers(int a, int b) { return a + b + *p; }
