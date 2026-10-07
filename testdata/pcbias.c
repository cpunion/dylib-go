static volatile char c;
static volatile short s;
static volatile int i;
int pcbias(int a, int b) { c=1; s=2; i=4; return a+b+c+s+i; }
