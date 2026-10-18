#include "exports.h"
#include <stdarg.h>

PlainWide echo_wide(PlainWide v) { v.wide_real+=1.5f; v.wide_tag++; return v; }
PlainLead echo_lead(PlainLead v) { v.lead_real+=1.5f; return v; }
PlainTail echo_tail(PlainTail v) { v.tail_real+=1.5f; return v; }
PlainPair echo_pair(PlainPair v) { v.pair_left+=1.5f; v.pair_right+=2.5f; return v; }
PlainMixed echo_mixed(PlainMixed v) { v.mixed_tag++; v.mixed_real+=1.5; return v; }
PlainPointer echo_pointer(PlainPointer v) { v.pointer_real+=1.5f; return v; }
PlainMultiple echo_multiple(PlainMultiple v) { v.multiple_tag++; v.multiple_real+=1.5f; return v; }
PlainArray echo_array(PlainArray v) { for(int i=0;i<2;i++)for(int j=0;j<2;j++)v.array_values[i][j]+=1.5f; return v; }
PlainEnvelope echo_envelope(PlainEnvelope v) {
    v.envelope_tag++; v.envelope_real+=1.5;
    for(int i=0;i<2;i++) { v.leads[i]=echo_lead(v.leads[i]); for(int j=0;j<2;j++)v.items[i][j]=echo_wide(v.items[i][j]); }
    return v;
}
PlainWide plain_variable(PlainWide v,...) {
    va_list ap; va_start(ap,v); v.wide_tag+=va_arg(ap,int); v.wide_real+=va_arg(ap,double); va_end(ap); return v;
}
PlainWide plain_overflow(long long a0,long long a1,long long a2,long long a3,long long a4,long long a5,long long a6,long long a7,double d0,double d1,double d2,double d3,double d4,double d5,double d6,double d7,PlainWide v) {
    if(a0+a1+a2+a3+a4+a5+a6+a7!=36 || d0+d1+d2+d3+d4+d5+d6+d7!=36) { v.wide_real=-1; return v; }
    return echo_wide(v);
}
PlainWide (*plain_factory(void))(PlainWide) { return echo_wide; }
PlainWide plain_forward(PlainWide (*entry)(PlainWide),PlainWide v) { return entry?entry(v):v; }
static PlainWide storage;
PlainWide *plain_pointer(void) { return &storage; }
PlainWide plain_read(PlainWide *v) { return *v; }
void plain_write(PlainWide *p,PlainWide v) { *p=v; }
PlainWide plain_record_variable(int marker,...) {
    va_list ap; va_start(ap,marker); PlainWide v=va_arg(ap,PlainWide); int tag=va_arg(ap,int); double real=va_arg(ap,double); va_end(ap);
    v.wide_tag+=tag; v.wide_real+=real; if(marker!=42)v.wide_real=-1; return v;
}
