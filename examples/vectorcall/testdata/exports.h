#ifndef DYLIB_VECTORCALL_EXPORTS_H
#define DYLIB_VECTORCALL_EXPORTS_H
#define VECTOR __attribute__((vectorcall))
#define API __declspec(dllexport)
typedef struct { int a, b; } Pair;
typedef struct { float a, b, c, d; } HFA;
typedef struct { double a[8]; } Large;
typedef int (VECTOR *Entry)(int, int);
typedef HFA (VECTOR *HFAEntry)(HFA);
API unsigned VECTOR vector_unsigned(unsigned);
API _Bool VECTOR vector_bool(_Bool);
API void VECTOR vector_void(void);
API int VECTOR vector_zero(void);
API int VECTOR vector_add(int, int);
API long long VECTOR vector_wide(signed char, unsigned char, short, unsigned short, long long, unsigned long long);
API double VECTOR vector_mixed(int, float, double, int, float, double, float, double, int);
API Pair VECTOR vector_pair(Pair);
API HFA VECTOR vector_hfa(HFA);
API Large VECTOR vector_large(Large);
API void * VECTOR vector_pointer(void *);
API Entry vector_factory(void);
API int vector_forward(Entry, int, int);
API int VECTOR vector_apply(Entry, int, int);
API HFAEntry vector_hfa_factory(void);
API HFA vector_hfa_forward(HFAEntry, HFA);
#endif
