#include "conventions.h"
int __attribute__((stdcall)) std_add(int a,int b) { return a+b; }
double __attribute__((stdcall)) std_mixed(int a,double b,float c,unsigned long long d) { return a+b+c+d; }
int __attribute__((fastcall)) fast_add(int a,int b) { return a+b; }
double __attribute__((fastcall)) fast_mixed(int a,int b,double c,float d) { return a+b+c+d; }
StdEntry std_factory(void) { return std_add; }
FastEntry fast_factory(void) { return fast_add; }
int apply_std(StdEntry entry,int a,int b) { return entry(a,b); }
int apply_fast(FastEntry entry,int a,int b) { return entry(a,b); }
int __attribute__((stdcall)) std_apply_fast(FastEntry entry,int a,int b) { return entry(a,b); }
int __attribute__((fastcall)) fast_apply_std(StdEntry entry,int a,int b) { return entry(a,b); }
