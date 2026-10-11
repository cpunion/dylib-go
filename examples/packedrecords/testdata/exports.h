typedef struct __attribute__((packed)) {
    signed char tag;
    double value;
    short tail;
    _Bool truth;
    void *context;
} Packed;
typedef struct { Packed items[2][2]; int values[2]; } Envelope;
Packed echo_packed(Packed);
Envelope echo_envelope(Envelope);
typedef Packed (*PackedFn)(Packed);
PackedFn packed_factory(void);
Packed apply_packed(PackedFn, Packed);
Packed *packed_pointer(void);
Packed *mutate_packed(Packed *);
double sum_packed(Packed *);
Packed packed_variable(Packed, ...);
