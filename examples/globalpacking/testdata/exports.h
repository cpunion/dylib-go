/* Compile this header and its exports with -fpack-struct=2. */
typedef struct { signed char tag; double real; int count; _Bool truth; void *context; } Packed;
typedef struct { signed char bits_tag; unsigned low:3; signed high:5; unsigned :0; _Bool active:1; unsigned long long wide:40; double samples[2]; void *bits_context; } Bits;
typedef struct __attribute__((aligned(32))) { float left; float right; } Aligned;
typedef struct { signed char envelope_tag; Packed items[2][2]; Bits bits[2]; Aligned floats[1]; int envelope_count; } Envelope;
#pragma pack(push,8)
typedef struct { signed char special_tag; double special_real; int special_count; } Override;
#pragma pack(pop)
typedef struct { signed char restored_tag; double restored_real; int restored_count; } Restored;
Packed echo_packed(Packed);
Bits echo_bits(Bits);
Aligned echo_aligned(Aligned);
Envelope echo_envelope(Envelope);
Override echo_override(Override);
Restored echo_restored(Restored);
typedef Packed (*PackedFn)(Packed);
PackedFn packed_factory(void);
Packed apply_packed(PackedFn,Packed);
Packed *packed_pointer(void);
Packed *mutate_packed(Packed *);
double sum_packed(Packed *);
Packed packed_variable(Packed,...);
typedef struct { signed char slot_tag; PackedFn entry; } Slot;
Packed apply_slot(Slot,Packed);
Bits *bits_pointer(void);
Bits *mutate_bits(Bits *);
double sum_bits(Bits *);
Bits bits_variable(Bits,...);
