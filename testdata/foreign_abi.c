#include "foreign_abi.h"
#define DYLIB_FOREIGN_DEFINE(prefix,cc) \
int32_t cc prefix##_add(int32_t a,int32_t b){return a+b;} \
int8_t cc prefix##_narrow(int8_t a){return a;} \
uint64_t cc prefix##_wide(uint64_t a){return a^UINT64_C(0x12345678fedcba98);} \
_Bool cc prefix##_boolean(_Bool a){return !a;} \
void *cc prefix##_pointer(void *a){return a;} \
void cc prefix##_mutate(int32_t *a,int32_t b){*a+=b;} \
int32_t *cc prefix##_slot(void){static int32_t value;value=20;return &value;} \
int32_t cc prefix##_read(int32_t *a){return *a;} \
double cc prefix##_mixed(int8_t a,uint16_t b,float c,double d,int64_t e){return a+b+c+d+e;} \
double cc prefix##_spill(int32_t a,int32_t b,int32_t c,int32_t d,int32_t e,int32_t f,int32_t g,int32_t h,double i,double j,double k,double l,double m,double n,double o,double p,double q,double r){return a+b+c+d+e+f+g+h+i+j+k+l+m+n+o+p+q+r;} \
ForeignPair cc prefix##_pair(ForeignPair p,int32_t n){p.a+=n;p.b+=n;return p;} \
ForeignLarge cc prefix##_large(ForeignLarge p,int64_t n){p.a+=n;p.b+=n;p.c+=n;p.d+=n;p.e+=n;return p;} \
prefix##_PairEntry prefix##_factory(void){return prefix##_pair;} \
ForeignPair prefix##_apply(prefix##_PairEntry entry,ForeignPair p,int32_t n){return entry(p,n);} \
ForeignPair cc prefix##_forward(prefix##_PairEntry entry,ForeignPair p,int32_t n){return entry(p,n);} \
ForeignLarge prefix##_apply_large(prefix##_LargeEntry entry,ForeignLarge p,int64_t n){return entry(p,n);}
DYLIB_FOREIGN_DEFINE(ms,MS64)
DYLIB_FOREIGN_DEFINE(sysv,SYS64)
