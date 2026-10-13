
typedef int Low __attribute__((aligned(1)));
typedef Low Chain;
typedef int High __attribute__((aligned(16)));
typedef High Down __attribute__((aligned(2)));
typedef double FloatLow __attribute__((aligned(1)));
typedef double FloatHigh __attribute__((aligned(16)));
typedef int Maximum __attribute__((aligned));
typedef void *Address __attribute__((aligned(1)));
typedef int (*Entry)(int) __attribute__((aligned(1)));
enum Choice { choice_min=-1, choice_max=42 };
typedef enum Choice ChoiceLow __attribute__((aligned(1)));
typedef struct { signed char low_tag; const Chain low_value; } LowRecord;
typedef struct { signed char high_tag; High high_value; double high_real; } HighRecord;
typedef struct { signed char down_tag; Down down_value; } DownRecord;
typedef struct { signed char array_tag; Low array_items[2][2]; } ArrayRecord;
typedef struct { signed char float_tag; FloatLow float_real; } FloatRecord;
typedef struct { FloatHigh float_left; FloatHigh float_right; } FloatHighRecord;
typedef struct { signed char max_tag; Maximum max_value; } MaximumRecord;
typedef struct { signed char address_tag; Address address_value; Entry address_entry; } AddressRecord;
typedef struct { signed char enum_tag; ChoiceLow enum_value; } EnumRecord;
typedef struct __attribute__((packed)) { signed char packed_tag; High packed_value __attribute__((aligned(8))); Low packed_tail; } PackedRecord;
#pragma pack(push,2)
typedef struct { signed char pragma_tag; High pragma_value; FloatLow pragma_real; } PragmaRecord;
#pragma pack(pop)
LowRecord echo_low(LowRecord); HighRecord echo_high(HighRecord); DownRecord echo_down(DownRecord);
ArrayRecord echo_array(ArrayRecord); FloatRecord echo_float(FloatRecord); FloatHighRecord echo_float_high(FloatHighRecord);
MaximumRecord echo_maximum(MaximumRecord); AddressRecord echo_address(AddressRecord); EnumRecord echo_enum(EnumRecord);
PackedRecord echo_packed(PackedRecord); PragmaRecord echo_pragma(PragmaRecord);

int scalar(Down, FloatLow, Address);
int low_variadic(LowRecord,...); int high_variadic(HighRecord,...);
AddressRecord make_address(void); int apply_address(AddressRecord);
HighRecord *mutate_high(HighRecord *);
typedef HighRecord (*HighEntry)(HighRecord);
HighEntry high_factory(void); HighRecord forward_high(HighEntry,HighRecord);

HighRecord *high_pointer(HighRecord); HighRecord read_high(HighRecord *);

/* Alignment belongs to the whole array, independently of its element type. */
typedef int PairLow[2] __attribute__((aligned(1)));
typedef int PairHigh[2] __attribute__((aligned(16)));
typedef PairHigh PairDown __attribute__((aligned(2)));
typedef int PairMaximum[2] __attribute__((aligned));
typedef Low Row[2] __attribute__((aligned(8)));
typedef Row Matrix[2] __attribute__((aligned(1)));
typedef Matrix MatrixChain;
typedef LowRecord RecordPair[2] __attribute__((aligned(1)));
typedef void *AddressPair[2] __attribute__((aligned(1)));
typedef int (*EntryPair[2])(int) __attribute__((aligned(1)));
typedef double FloatSingle[1] __attribute__((aligned(1)));
typedef struct { signed char whole_low_tag; PairLow whole_low_items; } WholeLowRecord;
typedef struct { signed char whole_high_tag; PairHigh whole_high_items; } WholeHighRecord;
typedef struct { signed char whole_down_tag; PairDown whole_down_items; } WholeDownRecord;
typedef struct { signed char layered_tag; MatrixChain layered_items[3]; } LayeredRecord;
typedef struct { signed char record_array_tag; RecordPair record_array_items; } RecordArrayRecord;
typedef struct { signed char address_array_tag; AddressPair address_array_values; EntryPair address_array_entries; } AddressArrayRecord;
typedef struct { signed char whole_float_tag; FloatSingle whole_float_items; } WholeFloatRecord;
typedef struct { signed char whole_max_tag; PairMaximum whole_max_items; } WholeMaximumRecord;
typedef struct __attribute__((packed)) { signed char whole_packed_tag; PairHigh whole_packed_items __attribute__((aligned(8))); Matrix whole_packed_matrix; } WholePackedRecord;
#pragma pack(push,2)
typedef struct { signed char whole_pragma_tag; PairHigh whole_pragma_items; FloatSingle whole_pragma_float; } WholePragmaRecord;
#pragma pack(pop)
WholeLowRecord echo_whole_low(WholeLowRecord); WholeHighRecord echo_whole_high(WholeHighRecord);
WholeDownRecord echo_whole_down(WholeDownRecord); LayeredRecord echo_layered(LayeredRecord);
RecordArrayRecord echo_record_array(RecordArrayRecord); AddressArrayRecord echo_address_array(AddressArrayRecord);
WholeFloatRecord echo_whole_float(WholeFloatRecord); WholeMaximumRecord echo_whole_maximum(WholeMaximumRecord);
WholePackedRecord echo_whole_packed(WholePackedRecord); WholePragmaRecord echo_whole_pragma(WholePragmaRecord);
int whole_variadic(WholeLowRecord,...);
typedef WholeHighRecord (*WholeEntry)(WholeHighRecord);
WholeEntry whole_factory(void); WholeHighRecord whole_forward(WholeEntry,WholeHighRecord);
WholeHighRecord *whole_pointer(WholeHighRecord); WholeHighRecord *whole_mutate(WholeHighRecord *);
WholeHighRecord whole_read(WholeHighRecord *);
AddressArrayRecord make_address_array(void); int apply_address_array(AddressArrayRecord);
int decay_pair(PairLow);
