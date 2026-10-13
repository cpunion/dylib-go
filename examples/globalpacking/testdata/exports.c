#include "exports.h"
#if defined(_WIN32)
int _fltused;
#endif
Packed echo_packed(Packed p) { p.tag++; p.real+=1.5; p.count+=2; p.truth=!p.truth; return p; }
Bits echo_bits(Bits b) { b.bits_tag++; b.low^=3; b.high--; b.active=!b.active; b.wide^=0x55; b.samples[0]+=1.5; b.samples[1]+=2.5; return b; }
Aligned echo_aligned(Aligned a) { a.left+=1.5f; a.right+=2.5f; return a; }
Envelope echo_envelope(Envelope e) { e.envelope_tag++; e.envelope_count+=2; for(int i=0;i<2;i++) { for(int j=0;j<2;j++) e.items[i][j]=echo_packed(e.items[i][j]); e.bits[i]=echo_bits(e.bits[i]); } e.floats[0]=echo_aligned(e.floats[0]); return e; }
Override echo_override(Override p) { p.special_tag++; p.special_real+=1.5; p.special_count+=2; return p; }
Restored echo_restored(Restored p) { p.restored_tag++; p.restored_real+=1.5; p.restored_count+=2; return p; }
PackedFn packed_factory(void) { return echo_packed; }
Packed apply_packed(PackedFn fn,Packed p) { return fn?fn(p):p; }
static Packed packed={-128,20.5,20,1,0};
Packed *packed_pointer(void) { packed.context=&packed; return &packed; }
Packed *mutate_packed(Packed *p) { if(p) *p=echo_packed(*p); return p; }
double sum_packed(Packed *p) { return p?p->real+p->count:-1; }
Packed packed_variable(Packed p,...) { __builtin_va_list args; __builtin_va_start(args,p); p.count+=__builtin_va_arg(args,int); p.real+=__builtin_va_arg(args,double); __builtin_va_end(args); return p; }
Packed apply_slot(Slot s,Packed p) { if(s.entry) p=s.entry(p); p.tag+=s.slot_tag; return p; }
static Bits bits={-128,7,-15,1,1099511627775ULL,{20.5,19.5},0};
Bits *bits_pointer(void) { bits.bits_context=&bits; return &bits; }
Bits *mutate_bits(Bits *b) { if(b) *b=echo_bits(*b); return b; }
double sum_bits(Bits *b) { return b?b->samples[0]+b->samples[1]:-1; }
Bits bits_variable(Bits b,...) { __builtin_va_list args; __builtin_va_start(args,b); b.high+=__builtin_va_arg(args,int); b.samples[0]+=__builtin_va_arg(args,double); __builtin_va_end(args); return b; }
