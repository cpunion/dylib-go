
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
typedef struct { char low_tag; const Chain low_value; } LowRecord;
typedef struct { char high_tag; High high_value; double high_real; } HighRecord;
typedef struct { char down_tag; Down down_value; } DownRecord;
typedef struct { char array_tag; Low array_items[2][2]; } ArrayRecord;
typedef struct { char float_tag; FloatLow float_real; } FloatRecord;
typedef struct { FloatHigh float_left; FloatHigh float_right; } FloatHighRecord;
typedef struct { char max_tag; Maximum max_value; } MaximumRecord;
typedef struct { char address_tag; Address address_value; Entry address_entry; } AddressRecord;
typedef struct { char enum_tag; ChoiceLow enum_value; } EnumRecord;
typedef struct __attribute__((packed)) { char packed_tag; High packed_value __attribute__((aligned(8))); Low packed_tail; } PackedRecord;
#pragma pack(push,2)
typedef struct { char pragma_tag; High pragma_value; FloatLow pragma_real; } PragmaRecord;
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
