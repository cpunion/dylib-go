#include "exports.h"
signed char s8(signed char x){return x+1;}
unsigned char u8(unsigned char x){return x-1;}
short s16(short x){return x+1;}
unsigned short u16(unsigned short x){return x-1;}
int s32(int x){return x+1;}
unsigned int u32(unsigned int x){return x-1;}
long long s64(long long x){return x+1;}
unsigned long long u64(unsigned long long x){return x-1;}
float f32(float x){return x+1.5f;}
double f64(double x){return x+1.5;}
_Bool truth(_Bool x){return !x;}
void *pointer(void *x){return x;}
int zero(void){return 42;}
void empty(void){}
double mixed(int a,double b,float c,unsigned long long d){return a+b+c+d;}
