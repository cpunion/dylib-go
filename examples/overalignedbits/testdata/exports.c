#include "exports.h"
#if defined(_WIN32)
int _fltused;
#endif
Tiny echo_tiny(Tiny t) { t.tiny_low^=3; t.tiny_high--; return t; }
Bits echo_bits(Bits b) { b.tag++; b.low^=3; b.high--; b.truth=!b.truth; b.wide^=0x55; b.real+=1.5; b.count+=2; return b; }
Padding echo_padding(Padding p) { p.pad_high--; p.samples[0]+=1.5f; p.samples[1]+=2.5f; return p; }
Inner echo_inner(Inner i) { i.inner_low^=3; i.inner_samples[0]+=1.5f; i.inner_samples[1]+=2.5f; return i; }
Envelope echo_envelope(Envelope e) { e.envelope_tag++; e.envelope_count+=2; for(int i=0;i<2;i++) { for(int j=0;j<2;j++) e.items[i][j]=echo_bits(e.items[i][j]); e.pads[i]=echo_padding(e.pads[i]); } e.inners[0]=echo_inner(e.inners[0]); return e; }
Floating echo_floating(Floating f) { f.left+=1.5f; f.right+=2.5f; return f; }
Packed echo_packed(Packed p) { p.packed_tag++; p.packed_high--; p.packed_real+=1.5; return p; }
PaddingFloat echo_padding_float(PaddingFloat f) { f.gap_left+=1.5f; f.gap_right+=2.5f; return f; }
BitsFn bits_factory(void) { return echo_bits; }
Bits apply_bits(BitsFn fn,Bits b) { return fn?fn(b):b; }
static Bits stored={-128,7,-15,1,1099511627775ULL,20.5,20,0};
Bits *bits_pointer(void) { stored.context=&stored; return &stored; }
Bits *mutate_bits(Bits *b) { if(b) *b=echo_bits(*b); return b; }
double sum_bits(Bits *b) { return b?b->real+b->count:-1; }
Bits bits_variable(Bits b,...) { __builtin_va_list a; __builtin_va_start(a,b); b.count+=__builtin_va_arg(a,int); b.real+=__builtin_va_arg(a,double); __builtin_va_end(a); return b; }
Tiny tiny_variable(Tiny t,...) { __builtin_va_list a; __builtin_va_start(a,t); t.tiny_high+=__builtin_va_arg(a,int); __builtin_va_end(a); return t; }
Floating floating_variable(Floating f,...) { __builtin_va_list a; __builtin_va_start(a,f); f.left+=__builtin_va_arg(a,int); f.right+=__builtin_va_arg(a,double); __builtin_va_end(a); return f; }
Bits apply_slot(Slot s,Bits b) { if(s.entry) b=s.entry(b); b.tag+=s.slot_tag; b.count+=s.slot_flags; return b; }
TinyTail echo_tiny_tail(TinyTail t) { t.tail_low^=3; t.tail_high--; return t; }
TinyTail tiny_tail_variable(TinyTail t,...) { __builtin_va_list a; __builtin_va_start(a,t); t.tail_high+=__builtin_va_arg(a,int); t.tail_low^=(unsigned)__builtin_va_arg(a,double); __builtin_va_end(a); return t; }
FloatingTriple echo_triple(FloatingTriple t) { t.triple_left+=1.5f; t.triple_middle+=2.5f; t.triple_right+=3.5f; return t; }
FloatingTriple triple_variable(FloatingTriple t,...) { __builtin_va_list a; __builtin_va_start(a,t); t.triple_left+=__builtin_va_arg(a,int); t.triple_middle+=__builtin_va_arg(a,double); __builtin_va_end(a); return t; }
FloatingSingle echo_single(FloatingSingle s) { s.single_left+=1.5f; return s; }
FloatingSingle single_variable(FloatingSingle s,...) { __builtin_va_list a; __builtin_va_start(a,s); s.single_left+=__builtin_va_arg(a,int); s.single_left+=__builtin_va_arg(a,double); __builtin_va_end(a); return s; }
FloatingBare echo_bare(FloatingBare b) { b.bare_left+=1.5f; return b; }
FloatingDouble echo_double(FloatingDouble d) { d.double_left+=1.5; return d; }
FloatingArray echo_array(FloatingArray a) { a.array_values[0]+=1.5f; return a; }
FloatingNested echo_nested(FloatingNested n) { n.nested.bare_left+=1.5f; return n; }
