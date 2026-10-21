#ifndef DYLIB_GO_FOREIGN_ABI_H
#define DYLIB_GO_FOREIGN_ABI_H
#include <stdint.h>
#define MS64 __attribute__((ms_abi))
#define SYS64 __attribute__((sysv_abi))
typedef struct {int32_t a; double b;} ForeignPair;
typedef struct {int64_t a,b,c,d,e;} ForeignLarge;
#define DYLIB_FOREIGN_DECLARE(prefix,cc) \
int32_t cc prefix##_add(int32_t,int32_t); \
int8_t cc prefix##_narrow(int8_t); \
uint64_t cc prefix##_wide(uint64_t); \
_Bool cc prefix##_boolean(_Bool); \
void *cc prefix##_pointer(void *); \
void cc prefix##_mutate(int32_t *,int32_t); \
int32_t *cc prefix##_slot(void); \
int32_t cc prefix##_read(int32_t *); \
double cc prefix##_mixed(int8_t,uint16_t,float,double,int64_t); \
double cc prefix##_spill(int32_t,int32_t,int32_t,int32_t,int32_t,int32_t,int32_t,int32_t,double,double,double,double,double,double,double,double,double,double); \
ForeignPair cc prefix##_pair(ForeignPair,int32_t); \
ForeignLarge cc prefix##_large(ForeignLarge,int64_t); \
typedef ForeignPair (cc *prefix##_PairEntry)(ForeignPair,int32_t); \
typedef ForeignLarge (cc *prefix##_LargeEntry)(ForeignLarge,int64_t); \
prefix##_PairEntry prefix##_factory(void); \
ForeignPair prefix##_apply(prefix##_PairEntry,ForeignPair,int32_t); \
ForeignPair cc prefix##_forward(prefix##_PairEntry,ForeignPair,int32_t); \
ForeignLarge prefix##_apply_large(prefix##_LargeEntry,ForeignLarge,int64_t);
DYLIB_FOREIGN_DECLARE(ms,MS64)
DYLIB_FOREIGN_DECLARE(sysv,SYS64)
#endif
