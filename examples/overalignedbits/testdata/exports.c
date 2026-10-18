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
GapFloat echo_gap_float(GapFloat v) { v.interior_left+=1.5f; v.interior_right+=2.5f; return v; }
GapFloat gap_float_variable(GapFloat v,...) { __builtin_va_list a; __builtin_va_start(a,v); v.interior_left+=__builtin_va_arg(a,int); v.interior_right+=__builtin_va_arg(a,double); __builtin_va_end(a); return v; }
GapTail echo_gap_tail(GapTail v) { v.tail_left+=1.5f; v.tail_right+=2.5f; return v; }
GapTail gap_tail_variable(GapTail v,...) { __builtin_va_list a; __builtin_va_start(a,v); v.tail_left+=__builtin_va_arg(a,int); v.tail_right+=__builtin_va_arg(a,double); __builtin_va_end(a); return v; }
GapBits echo_gap_bits(GapBits v) { v.gap_low^=3; v.gap_high--; v.bits_right+=1.5f; return v; }
GapBits gap_bits_variable(GapBits v,...) { __builtin_va_list a; __builtin_va_start(a,v); v.gap_high+=__builtin_va_arg(a,int); double d=__builtin_va_arg(a,double); v.gap_low^=(unsigned)d; v.bits_right+=d; __builtin_va_end(a); return v; }
GapByte echo_gap_byte(GapByte v) { v.gap_tag++; v.byte_right+=1.5f; return v; }
GapByte gap_byte_variable(GapByte v,...) { __builtin_va_list a; __builtin_va_start(a,v); v.gap_tag+=__builtin_va_arg(a,int); v.byte_right+=__builtin_va_arg(a,double); __builtin_va_end(a); return v; }
GapNested echo_gap_nested(GapNested v) { v.gap_nested.bare_left+=1.5f; v.nested_right+=2.5f; return v; }
GapNested gap_nested_variable(GapNested v,...) { __builtin_va_list a; __builtin_va_start(a,v); v.gap_nested.bare_left+=__builtin_va_arg(a,int); v.nested_right+=__builtin_va_arg(a,double); __builtin_va_end(a); return v; }
GapArray echo_gap_array(GapArray v) { v.gap_array[0]+=1.5f; v.array_right+=2.5f; return v; }
GapArray gap_array_variable(GapArray v,...) { __builtin_va_list a; __builtin_va_start(a,v); v.gap_array[0]+=__builtin_va_arg(a,int); v.array_right+=__builtin_va_arg(a,double); __builtin_va_end(a); return v; }
GapMultiple echo_gap_multiple(GapMultiple v) { v.first_tag++; v.next_code--; v.multi_right+=1.5f; return v; }
GapMultiple gap_multiple_variable(GapMultiple v,...) { __builtin_va_list a; __builtin_va_start(a,v); v.first_tag+=__builtin_va_arg(a,int); v.next_code--; v.multi_right+=__builtin_va_arg(a,double); __builtin_va_end(a); return v; }

BytePrefix echo_byte_prefix(BytePrefix v) { v.byte_tag++; v.byte_real+=1.5f; return v; }
BitPrefix echo_bit_prefix(BitPrefix v) { v.bit_flags^=3; v.bit_real+=1.5; return v; }
FloatPrefix echo_float_prefix(FloatPrefix v) { v.float_first+=1.5f; v.float_tag++; v.float_second+=2.5f; return v; }
MultiPrefix echo_multi_prefix(MultiPrefix v) { v.multi_tag++; v.multi_code--; v.multi_real+=1.5f; return v; }
TypedefPrefix echo_typedef_prefix(TypedefPrefix v) { v.typedef_tag++; v.typedef_real+=1.5; return v; }
PrefixEnvelope echo_prefix_envelope(PrefixEnvelope v) { v.prefix_items[0]=echo_byte_prefix(v.prefix_items[0]); v.prefix_items[1]=echo_byte_prefix(v.prefix_items[1]); v.prefix_bits=echo_bit_prefix(v.prefix_bits); return v; }
BitPrefix prefix_variable(BitPrefix v,...) { __builtin_va_list a; __builtin_va_start(a,v); v.bit_flags^=__builtin_va_arg(a,int); v.bit_real+=__builtin_va_arg(a,double); __builtin_va_end(a); return v; }
double prefix_overflow(long long a,long long b,long long c,long long d,long long e,long long f,double g,double h,double i,double j,double k,double l,double m,double n,BitPrefix v,long long tail) { return a+b+c+d+e+f+g+h+i+j+k+l+m+n+v.bit_flags+v.bit_real+tail; }
PrefixEntry prefix_factory(void) { return echo_bit_prefix; }
BitPrefix forward_prefix(PrefixEntry entry,BitPrefix v) { return entry?entry(v):v; }
static BitPrefix prefix_storage;
BitPrefix *prefix_pointer(BitPrefix v) { prefix_storage=v; return &prefix_storage; }
BitPrefix read_prefix(BitPrefix *v) { BitPrefix zero={0}; return v?*v:zero; }

PackedRootByte echo_root_byte(PackedRootByte v) { v.root_byte_tag++; v.root_byte_real+=1.5f; return v; }
PackedRootLeading echo_root_leading(PackedRootLeading v) { v.root_leading_tag++; v.root_leading_real+=1.5f; return v; }
PackedRootNamed echo_root_named(PackedRootNamed v) { v.root_named_flags^=3; v.root_named_tag++; v.root_named_real+=1.5f; return v; }
PackedRootFloating echo_root_floating(PackedRootFloating v) { v.root_floating_head+=1.5f; v.root_floating_tag++; v.root_floating_real+=2.5; return v; }
PackedRootAligned echo_root_aligned(PackedRootAligned v) { v.root_aligned_tag++; v.root_aligned_real+=1.5; return v; }
PackedRootEnvelope echo_root_envelope(PackedRootEnvelope v) { v.root_items[0]=echo_root_byte(v.root_items[0]); v.root_items[1]=echo_root_byte(v.root_items[1]); v.root_bits=echo_root_named(v.root_bits); v.root_floats=echo_root_floating(v.root_floats); return v; }
PackedRootByte root_variable(PackedRootByte v,...) { __builtin_va_list a; __builtin_va_start(a,v); v.root_byte_tag+=__builtin_va_arg(a,int); v.root_byte_real+=__builtin_va_arg(a,double); __builtin_va_end(a); return v; }
double root_overflow(long long a,long long b,long long c,long long d,long long e,long long f,double g,double h,double i,double j,double k,double l,double m,double n,PackedRootByte v,long long tail) { return a+b+c+d+e+f+g+h+i+j+k+l+m+n+v.root_byte_tag+v.root_byte_real+tail; }
RootEntry root_factory(void) { return echo_root_byte; }
PackedRootByte forward_root(RootEntry entry,PackedRootByte v) { return entry?entry(v):v; }
static PackedRootEnvelope root_storage;
PackedRootEnvelope *root_pointer(PackedRootEnvelope v) { root_storage=v; return &root_storage; }
PackedRootEnvelope read_root(PackedRootEnvelope *v) { PackedRootEnvelope zero={0}; return v?*v:zero; }

ClassWide echo_class_wide(ClassWide v) { v.class_wide_real+=1.5f; v.class_wide_tag++; return v; }
ClassNested echo_class_nested(ClassNested v) { v.class_integer.class_inner_integer++; v.class_nested_real+=1.5f; return v; }
ClassArray echo_class_array(ClassArray v) { v.class_items[0]++; v.class_array_real+=1.5f; return v; }
