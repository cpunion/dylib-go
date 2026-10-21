#ifdef RECORDS
struct big { long long a, b, c; };
#define PARAMETERS struct big value, double f0, double f1, double f2, double f3, double f4, double f5, double f6, double f7, double f8, double f9, int n0, int n1, int n2, int n3, int n4, int n5, int n6, int n7, int n8, int n9
#define ARGUMENTS value, 0.5, 1.5, 2.5, 3.5, 4.5, 5.5, 6.5, 7.5, 8.5, 9.5, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9

#ifndef CONSUMER_ONLY
struct big eval(PARAMETERS) {
    value.a += n0+n1+n2+n3+n4+n5+n6+n7+n8+n9;
    value.b += (long long)(f0+f1+f2+f3+f4+f5+f6+f7+f8+f9);
    value.c += 1;
    return value;
}

struct big other(PARAMETERS) {
    value.a += n0+n1+n2+n3+n4+n5+n6+n7+n8+n9;
    value.b += (long long)(f0+f1+f2+f3+f4+f5+f6+f7+f8+f9);
    value.c += 2;
    return value;
}
#endif

#ifndef PROVIDER_ONLY
extern struct big stub_eval(PARAMETERS);
extern struct big stub_other(PARAMETERS);
int consume_custom(int a, int b) {
    struct big value = {a, b, 3};
    struct big first = stub_eval(ARGUMENTS);
    struct big second = stub_other(ARGUMENTS);
    if (first.a != a+45 || first.b != b+50 || first.c != 4 ||
        second.a != a+45 || second.b != b+50 || second.c != 5) return -1;
    return (int)(first.a+first.b+first.c+second.a+second.b+second.c);
}
#endif
#else
#ifndef CONSUMER_ONLY
int eval(int a, int b) { return a+b; }
int other(int a, int b) { return a-b; }
#endif

#ifndef PROVIDER_ONLY
extern int stub_eval(int, int);
extern int stub_other(int, int);
int consume_custom(int a, int b) { return stub_eval(a,b)+stub_other(a,b); }
#endif
#endif
