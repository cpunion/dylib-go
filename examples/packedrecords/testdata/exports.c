#include "exports.h"
#include <stdarg.h>

static int context;
static Packed stored = { -128, 20.5, 22, 1, 0 };

Packed echo_packed(Packed p) {
    p.tag += 1;
    p.value += 1.5;
    p.tail -= 1;
    p.truth = !p.truth;
    return p;
}
Envelope echo_envelope(Envelope p) {
    for (int i = 0; i < 2; ++i)
        for (int j = 0; j < 2; ++j)
            p.items[i][j] = echo_packed(p.items[i][j]);
    p.values[0] += 1;
    p.values[1] += 2;
    return p;
}
PackedFn packed_factory(void) { return echo_packed; }
Packed apply_packed(PackedFn fn, Packed p) { return fn(p); }
Packed *packed_pointer(void) { stored.context = &context; return &stored; }
Packed *mutate_packed(Packed *p) {
    if (p) *p = echo_packed(*p);
    return p;
}
double sum_packed(Packed *p) { return p ? p->value + p->tail : -1; }
Packed packed_variable(Packed p, ...) {
    va_list args;
    va_start(args, p);
    p.tail += va_arg(args, int);
    p.value += va_arg(args, double);
    va_end(args);
    return p;
}

static Member member_stored={-128,{20.5,19.5},22,0};
Member echo_member(Member p){p.tag++;p.values[0]+=1.5;p.values[1]+=2.5;p.tail--;return p;}
MemberEnvelope echo_members(MemberEnvelope p){for(int i=0;i<2;i++)for(int j=0;j<2;j++)p.items[i][j]=echo_member(p.items[i][j]);p.count++;return p;}
MemberAligned echo_member_aligned(MemberAligned p){p.marker++;p.samples[0]+=1.5;p.samples[1]+=2.5;p.counter+=2;p.truth=!p.truth;return p;}
MemberFn member_factory(void){return echo_member;}
Member apply_member(MemberFn fn,Member p){return fn(p);}
Member *member_pointer(void){member_stored.context=&context;return &member_stored;}
Member *mutate_member(Member *p){if(p)*p=echo_member(*p);return p;}
double sum_member(Member *p){return p?p->values[0]+p->tail:-1;}
Member member_variable(Member p,...){va_list args;va_start(args,p);p.tail+=va_arg(args,int);p.values[0]+=va_arg(args,double);va_end(args);return p;}

static Pragma pragma_stored={-128,{20.5,19.5},22,1,0};
Pragma echo_pragma(Pragma p){p.pragma_tag++;p.pragma_samples[0]+=1.5;p.pragma_samples[1]+=2.5;p.pragma_count++;p.pragma_truth=!p.pragma_truth;return p;}
PragmaEnvelope echo_pragma_envelope(PragmaEnvelope p){for(int i=0;i<2;i++)for(int j=0;j<2;j++)p.pragma_items[i][j]=echo_pragma(p.pragma_items[i][j]);p.envelope_tag++;p.checksum+=2;return p;}
PragmaAligned echo_pragma_aligned(PragmaAligned p){p.aligned_tag++;p.aligned_count+=2;p.aligned_value+=1.5;return p;}
PragmaOuter echo_pragma_outer(PragmaOuter p){p.payload=echo_pragma_envelope(p.payload);p.natural_tail+=2.5;return p;}
PragmaFn pragma_factory(void){return echo_pragma;}
Pragma apply_pragma(PragmaFn fn,Pragma p){return fn(p);}
Pragma *pragma_pointer(void){pragma_stored.pragma_context=&context;return &pragma_stored;}
Pragma *mutate_pragma(Pragma *p){if(p)*p=echo_pragma(*p);return p;}
double sum_pragma(Pragma *p){return p?p->pragma_samples[0]+p->pragma_count:-1;}
Pragma pragma_variable(Pragma p,...){va_list args;va_start(args,p);p.pragma_count+=va_arg(args,int);p.pragma_samples[0]+=va_arg(args,double);va_end(args);return p;}
Pragma apply_pragma_slot(PragmaSlot p,Pragma v){v.pragma_tag+=p.slot_tag;return p.entry?p.entry(v):v;}
