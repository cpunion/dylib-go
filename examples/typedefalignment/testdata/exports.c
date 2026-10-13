#include "exports.h"
#if defined(_MSC_VER)
int _fltused = 0x9875;
#endif
#define ECHO(T,N) T N(T value){ return value; }
ECHO(LowRecord,echo_low) ECHO(HighRecord,echo_high) ECHO(DownRecord,echo_down)
ECHO(ArrayRecord,echo_array) ECHO(FloatRecord,echo_float) ECHO(FloatHighRecord,echo_float_high)
ECHO(MaximumRecord,echo_maximum) ECHO(AddressRecord,echo_address) ECHO(EnumRecord,echo_enum)
ECHO(PackedRecord,echo_packed) ECHO(PragmaRecord,echo_pragma)
int scalar(Down a, FloatLow b, Address c){return a+(int)b+(c!=0);}
#define VAR(T,N,V) int N(T value,...){__builtin_va_list ap;__builtin_va_start(ap,value);int a=__builtin_va_arg(ap,int);double b=__builtin_va_arg(ap,double);__builtin_va_end(ap);return (V)+a+(int)b;}
VAR(LowRecord,low_variadic,value.low_tag+value.low_value)
VAR(HighRecord,high_variadic,value.high_tag+value.high_value+(int)value.high_real)
static int native_value=20;
static int entry(int x){return x+22;}
AddressRecord make_address(void){AddressRecord a;a.address_tag=1;a.address_value=&native_value;a.address_entry=entry;return a;}
int apply_address(AddressRecord a){return *(int*)a.address_value+a.address_entry(0);}
HighRecord *mutate_high(HighRecord *v){v->high_value+=1;v->high_real+=2;return v;}
HighEntry high_factory(void){return echo_high;}
HighRecord forward_high(HighEntry entry,HighRecord v){return entry(v);}

static HighRecord high_storage;
HighRecord *high_pointer(HighRecord v){ high_storage=v;return &high_storage; }
HighRecord read_high(HighRecord *v){return *v;}
