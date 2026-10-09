extern int datum[2];
extern int via_table(int, int);
static int local_value = 42;
int large_call(int a, int b) {
    return via_table(a, b) + datum[1] + local_value;
}
