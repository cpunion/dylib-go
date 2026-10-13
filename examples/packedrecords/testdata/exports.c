#include "exports.h"
#include <stdarg.h>

#if defined(_WIN32) && defined(_MSC_VER)
// Keep the MSVC floating-point marker when a MinGW driver links this object.
int _fltused = 0x9875;
#endif

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

static PragmaOver over_stored={-128,20,{20.5,19.5},1,0};
PragmaOver echo_over(PragmaOver p){p.over_tag++;p.over_count+=2;p.over_samples[0]+=1.5;p.over_samples[1]+=2.5;p.over_truth=!p.over_truth;return p;}
PragmaOverPacked echo_over_packed(PragmaOverPacked p){p.packed_tag++;p.packed_count+=2;p.packed_real+=1.5;return p;}
PragmaOverMax echo_over_max(PragmaOverMax p){p.max_tag++;p.max_count+=2;p.max_real+=1.5;return p;}
PragmaOverType echo_over_type(PragmaOverType p){p.type_tag++;p.type_count+=2;p.type_real+=1.5;return p;}
PragmaOverArray echo_over_array(PragmaOverArray p){p.array_tag++;p.array_values[0]+=1.5;p.array_values[1]+=2.5;p.array_count+=2;return p;}
PragmaOverEnvelope echo_over_envelope(PragmaOverEnvelope p){
    p.nested_tag++;p.nested_count+=2;
    for(int i=0;i<2;i++){
        for(int j=0;j<2;j++)p.nested_items[i][j]=echo_over(p.nested_items[i][j]);
        p.nested_packed[i]=echo_over_packed(p.nested_packed[i]);
        p.nested_max[i]=echo_over_max(p.nested_max[i]);
        p.nested_type[i]=echo_over_type(p.nested_type[i]);
        p.nested_array[i]=echo_over_array(p.nested_array[i]);
        p.nested_aligned[i]=echo_pragma_aligned(p.nested_aligned[i]);
    }
    return p;
}
PragmaOverFloat echo_over_float(PragmaOverFloat p){p.over_left+=1.5f;p.over_right+=2.5f;return p;}
PragmaOverOuter echo_over_outer(PragmaOverOuter p){p.over_payload=echo_over_envelope(p.over_payload);p.over_tail+=2.5;return p;}
PragmaOverFn over_factory(void){return echo_over;}
PragmaOver apply_over(PragmaOverFn fn,PragmaOver p){return fn(p);}
PragmaOver *over_pointer(void){over_stored.over_context=&context;return &over_stored;}
PragmaOver *mutate_over(PragmaOver *p){if(p)*p=echo_over(*p);return p;}
double sum_over(PragmaOver *p){return p?p->over_samples[0]+p->over_count:-1;}
PragmaOver over_variable(PragmaOver p,...){va_list args;va_start(args,p);p.over_count+=va_arg(args,int);p.over_samples[0]+=va_arg(args,double);va_end(args);return p;}
PragmaOver apply_over_slot(PragmaOverSlot p,PragmaOver v){v.over_tag+=p.over_slot_tag;return p.over_entry?p.over_entry(v):v;}
PragmaOverFloatNested echo_over_float_nested(PragmaOverFloatNested p){p.over_floats[0]=echo_over_float(p.over_floats[0]);p.over_float_tail+=3.5f;return p;}
