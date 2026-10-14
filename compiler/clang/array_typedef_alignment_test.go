package clang

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

var arrayTypedefFunctions = []string{"echo_whole_low", "echo_whole_high", "echo_whole_down", "echo_layered", "echo_record_array", "echo_address_array", "echo_whole_float", "echo_whole_maximum", "echo_whole_packed", "echo_whole_pragma"}

func arrayTypedefFixture(t *testing.T) (string, []string) {
	t.Helper()
	data, err := os.ReadFile("../../examples/typedefalignment/testdata/exports.h")
	if err != nil {
		t.Fatal(err)
	}
	names := append(append([]string(nil), scalarTypedefFunctions...), arrayTypedefFunctions...)
	return string(data), names
}

func TestArrayTypedefAlignmentAcrossTargets(t *testing.T) {
	source, names := arrayTypedefFixture(t)
	compiler, path := typedefCompilerTool(t), headerFile(t, source)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: names})
			if errors.Is(err, errTypedefABI) {
				t.Log("compiler rejects legacy SysV typedef ABI:", err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, test := range []struct {
				name       string
				depths     []int
				alignments []uint64
			}{
				{"WholeLowRecord", []int{1}, []uint64{1}},
				{"WholeHighRecord", []int{1}, []uint64{16}},
				{"WholeDownRecord", []int{1}, []uint64{2}},
				{"LayeredRecord", []int{0, 1, 2}, []uint64{1, 8, 1}},
				{"RecordArrayRecord", []int{1}, []uint64{1}},
				{"WholeFloatRecord", []int{1}, []uint64{1}},
			} {
				r, err := h.LookupRecord(test.name)
				if err != nil || len(r.FieldTypeAlignments) != len(test.depths) {
					t.Fatal(test.name, r, err)
				}
				for i, field := range r.FieldTypeAlignments {
					if field.Position != 1 || field.ArrayDepth != test.depths[i] || field.Aligned != test.alignments[i] || field.StorageAlignment == 0 {
						t.Fatal("alignment applied to the wrong array layer", test, r)
					}
				}
			}
			layered, _ := h.LookupRecord("LayeredRecord")
			dimensions, _ := arrayDimensions(layered.Description.Fields[1].Type)
			if !reflect.DeepEqual(dimensions, []int{3, 2, 2}) {
				t.Fatal("array dimensions lost through aliases", dimensions)
			}
			address, _ := h.LookupRecord("AddressArrayRecord")
			if len(address.FunctionPointers) != 1 || address.FunctionPointers[0].Position != 2 || address.FunctionPointers[0].Signature.Result != abi.I32 {
				t.Fatal("array entry prototype lost", address)
			}
			layered.FieldTypeAlignments[1].ArrayDepth = 3
			fresh, _ := h.LookupRecord("LayeredRecord")
			if fresh.FieldTypeAlignments[1].ArrayDepth != 1 {
				t.Fatal("array metadata snapshot shared")
			}
			if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
				t.Fatal("array typedef record accepted without a C bridge")
			}
		})
	}
}

func TestArrayTypedefAlignmentBridgeLayouts(t *testing.T) {
	source, names := arrayTypedefFixture(t)
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
		if target.os == "windows" {
			producers = append(producers, strings.Replace(target.triple, "pc-windows-msvc", "w64-windows-gnu", 1))
		}
	}
	for _, flags := range [][]string{nil, {"-fpack-struct=1"}} {
		t.Run(fmt.Sprint(flags), func(t *testing.T) {
			testCompilerBridgeTargets(t, typedefCompilerTool(t), source, flags, producers, names...)
		})
	}
}

func TestArrayTypedefAlignmentCallingABI(t *testing.T) {
	source, names := arrayTypedefFixture(t)
	path, compiler := headerFile(t, source), typedefCompilerTool(t)
	for _, target := range targets {
		if target.os == "windows" && target.arch == "386" {
			continue // MSVC argument/result adaptation is exercised by native CI.
		}
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: names})
			if errors.Is(err, errTypedefABI) {
				t.Log("compiler rejects legacy SysV typedef ABI:", err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			testCompilerRecordCallingABI(t, compiler, path, h)
		})
	}
}

func TestArrayTypedefAlignmentValidation(t *testing.T) {
	i32 := abi.TypeDesc{Type: abi.I32}
	array := abi.TypeDesc{Type: abi.Array, Len: 2, Elem: &i32}
	r := Record{Name: "S", Description: abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "value", Type: array}}}, Layout: abi.Layout{Size: 8, Alignment: 1, Offsets: []uint64{0}}}
	for _, fields := range [][]TypedefFieldAlignment{
		{{Position: 0, ArrayDepth: -1, Aligned: 1}},
		{{Position: 0, ArrayDepth: 2, Aligned: 1}},
		{{Position: 0, ArrayDepth: 1, Aligned: 1}, {Position: 0, Aligned: 1}},
		{{Position: 0, ArrayDepth: 1, Aligned: 1}, {Position: 0, ArrayDepth: 1, Aligned: 2}},
		{{Position: 0, Aligned: 1, StorageAlignment: 1}, {Position: 0, ArrayDepth: 1, Aligned: 1, StorageAlignment: 2}},
	} {
		r.FieldTypeAlignments = fields
		if err := (&Header{Records: []Record{r}}).validateDescriptions(); err == nil {
			t.Fatal("invalid array typedef metadata accepted", fields)
		}
	}
	r.FieldTypeAlignments = []TypedefFieldAlignment{{Position: 0, Aligned: 1, StorageAlignment: 1}, {Position: 0, ArrayDepth: 1, Aligned: 1, StorageAlignment: 1}}
	if err := validRecordLayout(r); err != nil {
		t.Fatal("valid multi-layer array metadata rejected", err)
	}
	for _, source := range []string{
		`typedef int Element __attribute__((aligned(16))); struct S { Element values[2]; }; struct S f(struct S);`,
		`typedef int Row[] __attribute__((aligned(1))); struct S { Row values; }; struct S f(struct S);`,
		`typedef int Row[2] __attribute__((aligned(1),deprecated)); typedef Row Alias; struct S { Alias values; }; struct S f(struct S);`,
	} {
		if h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: typedefCompilerTool(t), Functions: []string{"f"}}); err == nil {
			t.Fatal("invalid or unsupported array alias accepted", h)
		}
	}
}

func TestArrayTypedefIncompatibleStride(t *testing.T) {
	for _, element := range []string{"int Row[2]", "double Row[1]", "void *Row[1]", "Element Row[2]"} {
		t.Run(element, func(t *testing.T) {
			source := fmt.Sprintf("typedef struct { int value; } Element; typedef %s __attribute__((aligned(16))); struct S { Row values[2]; }; struct S f(struct S);", element)
			h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: typedefCompilerTool(t), Target: "aarch64-unknown-linux-gnu", Functions: []string{"f"}})
			if err != nil {
				t.Fatal(err)
			}
			if data, err := h.CgoSource("bindings", "Bindings"); err == nil || data != nil || !strings.Contains(err.Error(), "element stride") {
				t.Fatal("GCC-incompatible array stride was emitted", err)
			}
		})
	}
}

func TestArrayTypedefEnumBridgeLayouts(t *testing.T) {
	source := `enum Choice { first=-1, last=42 };
typedef enum Choice Low __attribute__((aligned(1)));
typedef Low Pair[2] __attribute__((aligned(2)));
typedef struct { signed char tag; Pair values; } S;
S f(S);`
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	testCompilerBridgeTargets(t, typedefCompilerTool(t), source, nil, producers, "f")
}

func TestArrayTypedefScalarPointerDecay(t *testing.T) {
	source := `typedef int Pair[2] __attribute__((aligned(1))); int sum(Pair);`
	h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: typedefCompilerTool(t), Functions: []string{"sum"}})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := h.Lookup("sum")
	if len(h.Records) != 0 || d.Signature.Result != abi.I32 || !reflect.DeepEqual(d.Signature.Args, []abi.Type{abi.Pointer}) {
		t.Fatal("array parameter failed to decay to a native address", h)
	}
	if _, err := h.CgoSource("bindings", "Bindings"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.LLGoSource("bindings", "Bindings"); err != nil && h.Target.Arch != "386" {
		t.Fatal(err)
	}
}
