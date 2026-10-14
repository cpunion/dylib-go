package clang

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

var recordTypedefFunctions = []string{"echo_plain", "echo_high", "echo_low", "echo_inherited", "echo_wide", "echo_wide_low", "echo_float", "echo_float_low", "echo_low_parent", "echo_high_parent", "echo_down_parent", "echo_wide_parent", "echo_float_parent", "echo_array_parent", "echo_packed_parent", "echo_pragma_parent", "echo_member_parent", "shifted", "overflow", "high_factory", "forward_high", "high_pointer", "read_high", "high_alignment", "wide_overflow", "alias_variadic", "echo_small_parent", "shifted_small"}

func recordTypedefFixture(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("../../examples/recordtypedefs/testdata/exports.h")
	if err != nil {
		t.Fatal(err)
	}
	return string(source)
}

func TestRecordTypedefBridge(t *testing.T) {
	source, compiler := recordTypedefFixture(t), typedefCompilerTool(t)
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
		if target.os == "windows" {
			producers = append(producers, strings.Replace(target.triple, "pc-windows-msvc", "w64-windows-gnu", 1))
		}
	}
	for _, flags := range [][]string{nil, {"-fpack-struct=1"}} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			testCompilerBridgeTargetsWithInspection(t, compiler, source, flags, producers, func(t *testing.T, path string, h *Header) {
				plain, err := h.LookupRecord("Plain")
				if err != nil || len(h.Records) != 14 || plain.Layout.Size != 4 || !reflect.DeepEqual(plain.TypedefAlignments, []RecordTypedefAlignment{{Name: "Down", Aligned: 2}, {Name: "High", Aligned: 16}, {Name: "Inherited", Aligned: 16}, {Name: "Low", Aligned: 1}}) {
					t.Fatal("alias requests changed the body layout", plain, err)
				}
				plain.TypedefAlignments[0].Aligned = 32
				fresh, _ := h.LookupRecord("Plain")
				if fresh.TypedefAlignments[0].Aligned != 2 {
					t.Fatal("alias metadata snapshot shared")
				}
				if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
					t.Fatal("aligned aliases accepted without a C bridge")
				}
				if err := h.validateHostRecords(h.Functions[0].Signature); err == nil || !strings.Contains(err.Error(), "compiled C adapter") {
					t.Fatal("aligned aliases accepted as ordinary native storage", err)
				}
				low, err := h.LookupRecord("LowParent")
				if err != nil || len(low.FieldTypeAlignments) != 1 || low.FieldTypeAlignments[0].Aligned != 1 {
					t.Fatal("record member alias lost", low, err)
				}
				if len(flags) == 0 && !strings.Contains(h.Target.Triple, "windows-gnu") {
					want := abi.Layout{Size: 6, Alignment: 1, Offsets: []uint64{0, 1, 5}}
					if h.Target.OS == "windows" {
						want = abi.Layout{Size: 12, Alignment: 4, Offsets: []uint64{0, 4, 8}}
					}
					if !reflect.DeepEqual(low.Layout, want) {
						t.Fatal("producer placement lost", low.Layout, want)
					}
				}
				if len(flags) == 0 && !(h.Target.OS == "windows" && h.Target.Arch == "386") && !strings.Contains(h.Target.Triple, "windows-gnu") {
					testCompilerRecordCallingABI(t, compiler, path, h)
				}
			}, recordTypedefFunctions...)
		})
	}
}

func TestRecordTypedefGCCBridge(t *testing.T) {
	testCompilerRecordGCCBridge(t, typedefCompilerTool(t), recordTypedefFixture(t), recordTypedefFunctions)
}

func TestRecordTypedefValidation(t *testing.T) {
	r := Record{Name: "Plain", Description: abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "value", Type: abi.TypeDesc{Type: abi.I32}}}}, Layout: abi.Layout{Size: 4, Alignment: 4, Offsets: []uint64{0}}}
	for _, aliases := range [][]RecordTypedefAlignment{
		{{Name: "High", Aligned: 0}}, {{Name: "High", Aligned: 3}}, {{Name: "bad name", Aligned: 16}},
		{{Name: "High", Aligned: 16}, {Name: "High", Aligned: 1}}, {{Name: "Low", Aligned: 1}, {Name: "High", Aligned: 16}},
	} {
		r.TypedefAlignments = aliases
		if err := (&Header{Records: []Record{r}}).validateDescriptions(); err == nil {
			t.Fatal("invalid alias metadata accepted", aliases)
		}
	}
	r.TypedefAlignments = []RecordTypedefAlignment{{Name: "High", Aligned: 16}}
	if err := validRecordLayout(r); err != nil {
		t.Fatal("alignment larger than alias size rejected", err)
	}
	other := r
	other.Name = "Other"
	if err := (&Header{Records: []Record{r, other}}).validateDescriptions(); err == nil {
		t.Fatal("duplicate alias names on distinct bodies accepted")
	}
	for _, source := range []string{
		`typedef struct { int value; } High __attribute__((aligned(16))); typedef High Alias; Alias f(Alias);`,
		`typedef struct { int value; } Plain; typedef Plain High __attribute__((aligned(16))); struct S { High values[2]; }; struct S f(struct S);`,
		`typedef union { int value; float scalar; } U; typedef U High __attribute__((aligned(16))); High f(High);`,
	} {
		_, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: typedefCompilerTool(t), Functions: []string{"f"}})
		if err == nil || errors.Is(err, errTypedefABI) {
			t.Fatal("invalid stride or union accepted", err)
		}
	}
}

func TestNamedRecordTypedefBody(t *testing.T) {
	source := `struct S { int value; };
typedef struct S High __attribute__((aligned));
typedef High Alias;
Alias *pointer(Alias *);`
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: typedefCompilerTool(t), Target: target.triple, Functions: []string{"pointer"}})
			if err != nil {
				t.Fatal(err)
			}
			r, err := h.LookupRecord("struct S")
			if err != nil || r.Layout.Size != 4 || r.Layout.Alignment != 4 || !reflect.DeepEqual(r.TypedefAlignments, []RecordTypedefAlignment{{Name: "Alias", Aligned: 16}}) {
				t.Fatal("pointer-only inherited maximum alignment lost", r, err)
			}
		})
	}
}
