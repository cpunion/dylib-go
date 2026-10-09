struct big { long long a, b, c; };

struct big eval(struct big value, double delta) {
    value.c = (long long)delta;
    return value;
}

extern struct big stub_eval(struct big, double);
extern struct big (*lazy_eval)(struct big, double);
extern struct big (*nonlazy_eval)(struct big, double);

static int sum(struct big value) { return (int)(value.a + value.b + value.c); }

int consume_records(int a, int b) {
    struct big value = {a, b, 0};
    return sum(stub_eval(value, 2.0)) + sum(lazy_eval(value, 2.0)) + sum(nonlazy_eval(value, 2.0));
}
