package clang

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

const scalarTypedefHeader = `
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
`

var scalarTypedefFunctions = []string{"echo_low", "echo_high", "echo_down", "echo_array", "echo_float", "echo_float_high", "echo_maximum", "echo_address", "echo_enum", "echo_packed", "echo_pragma"}

func TestScalarTypedefAlignmentAcrossTargets(t *testing.T) {
	compiler, path := compilerTool(t), headerFile(t, scalarTypedefHeader)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: scalarTypedefFunctions})
			if err != nil {
				t.Fatal(err)
			}
			for _, test := range []struct {
				name                    string
				alignment, size, offset uint64
			}{
				{"LowRecord", 1, 5, 1}, {"HighRecord", 16, 32, 16},
				{"DownRecord", 2, 6, 2}, {"ArrayRecord", 1, 17, 1},
				{"FloatRecord", 1, 9, 1}, {"EnumRecord", 1, 5, 1},
			} {
				r, err := h.LookupRecord(test.name)
				wantAlignment, wantSize, wantOffset := test.alignment, test.size, test.offset
				if target.os == "windows" && (test.name == "LowRecord" || test.name == "DownRecord" || test.name == "EnumRecord") {
					wantAlignment, wantSize, wantOffset = 4, 8, 4
				}
				if target.os == "windows" && test.name == "FloatRecord" {
					wantAlignment, wantSize, wantOffset = 8, 16, 8
				}
				if err != nil || r.Layout.Alignment != wantAlignment || r.Layout.Size != wantSize || r.Layout.Offsets[1] != wantOffset || r.Aligned != 0 {
					t.Fatal("typedef size/alignment was replaced by a member attribute", test, r, err)
				}
				if len(r.FieldTypeAlignments) == 0 || r.FieldTypeAlignments[0].Aligned != test.alignment || len(r.FieldAlignments) != 0 {
					t.Fatal("missing distinct typedef metadata", r)
				}
			}
			down, _ := h.LookupRecord("DownRecord")
			down.FieldTypeAlignments[0].Aligned = 32
			fresh, _ := h.LookupRecord("DownRecord")
			if fresh.FieldTypeAlignments[0].Aligned != 2 {
				t.Fatal("typedef metadata snapshot is shared")
			}
			address, _ := h.LookupRecord("AddressRecord")
			if len(address.FunctionPointers) != 1 || address.FunctionPointers[0].Position != 2 {
				t.Fatal("aligned native entry lost its prototype", address)
			}
			generated, err := h.CgoSource("bindings", "Bindings")
			if err != nil || !strings.Contains(string(generated), "FieldTypeAlignments:") {
				t.Fatal("typedef declarations or metadata missing", err)
			}
			if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
				t.Fatal("aligned typedef record accepted without a C bridge")
			}
		})
	}
}

func TestScalarTypedefAlignmentBridgeLayouts(t *testing.T) {
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
		if target.os == "windows" {
			producers = append(producers, strings.Replace(target.triple, "pc-windows-msvc", "w64-windows-gnu", 1))
		}
	}
	testBitfieldBridgeTargets(t, scalarTypedefHeader, nil, producers, scalarTypedefFunctions...)
}

func TestScalarTypedefDefaultPackingLayouts(t *testing.T) {
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	testBitfieldBridgeTargets(t, scalarTypedefHeader, []string{"-fpack-struct=1"}, producers, scalarTypedefFunctions...)
}

func TestScalarTypedefBoundaryAlignment(t *testing.T) {
	path := headerFile(t, scalarTypedefHeader+`High scalar(Down,FloatLow,Address); Entry factory(void); int apply(High (*)(Down));`)
	h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Functions: []string{"scalar", "factory", "apply"}})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := h.Lookup("scalar")
	if !reflect.DeepEqual(s.Signature.Args, []abi.Type{abi.I32, abi.F64, abi.Pointer}) || s.Signature.Result != abi.I32 {
		t.Fatal("scalar typedef changed the boundary ABI", s)
	}
	apply, _ := h.Lookup("apply")
	pointer, err := apply.LookupFunctionPointer(0)
	if err != nil || pointer.Signature.Result != abi.I32 || !reflect.DeepEqual(pointer.Signature.Args, []abi.Type{abi.I32}) {
		t.Fatal("aligned scalar callback prototype", pointer, err)
	}
	if _, err := h.CgoSource("bindings", "Bindings"); err != nil {
		t.Fatal("scalar C bridge", err)
	}
	if _, err := h.LLGoSource("bindings", "Bindings"); err != nil && h.Target.Arch != "386" {
		t.Fatal("scalar boundaries unnecessarily require a C bridge", err)
	}
}

func TestScalarTypedefAlignmentValidation(t *testing.T) {
	base := Record{Name: "S", Description: abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "value", Type: abi.TypeDesc{Type: abi.I32}}}}, Layout: abi.Layout{Size: 4, Alignment: 1, Offsets: []uint64{0}}}
	for _, fields := range [][]TypedefFieldAlignment{{{Position: -1, Aligned: 1}}, {{Position: 1, Aligned: 1}}, {{Position: 0, Aligned: 0}}, {{Position: 0, Aligned: 3}}, {{Position: 0, Aligned: 131072}}, {{Position: 0, Aligned: 1}, {Position: 0, Aligned: 2}}} {
		r := base
		r.FieldTypeAlignments = fields
		if err := (&Header{Records: []Record{r}}).validateDescriptions(); err == nil {
			t.Fatal("invalid typedef field metadata accepted", fields)
		}
	}
	base.FieldTypeAlignments = []TypedefFieldAlignment{{Position: 0, Aligned: 1, StorageAlignment: 1}}
	other := base
	other.FieldTypeAlignments = []TypedefFieldAlignment{{Position: 0, Aligned: 2, StorageAlignment: 1}}
	if _, err := newDirectTypes([]Record{base, other}, "Bindings"); err == nil {
		t.Fatal("distinct typedef ABI types collapsed")
	}
	for _, storage := range []uint64{0, 2, 3, 131072} {
		invalid := base
		invalid.FieldTypeAlignments = []TypedefFieldAlignment{{Position: 0, Aligned: 1, StorageAlignment: storage}}
		if err := validRecordLayout(invalid); err == nil {
			t.Fatal("invalid native typedef alignment accepted", storage)
		}
	}
	invalid := base
	invalid.Layout.Offsets = nil
	if err := validRecordLayout(invalid); err == nil {
		t.Fatal("missing native typedef offsets accepted")
	}
	invalid = base
	invalid.Bitfields = []Bitfield{{Position: 0, Width: 3}}
	if err := (&Header{Records: []Record{invalid}}).validateDescriptions(); err == nil {
		t.Fatal("typedef bitfield metadata accepted")
	}
	for _, source := range []string{
		`typedef struct {int value;} S __attribute__((aligned(16))); S f(S);`,
		`typedef int A[2] __attribute__((aligned(1))); struct S { A value; }; struct S f(struct S);`,
		`typedef int I __attribute__((aligned(1))); struct S { I value:3; }; struct S f(struct S);`,
		`typedef int I __attribute__((aligned(1),deprecated)); typedef I J; J f(J);`,
	} {
		if h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: compilerTool(t), Functions: []string{"f"}}); err == nil {
			t.Fatal("unsupported typedef form accepted", h)
		}
	}
}
