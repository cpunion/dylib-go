package clang

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func pragmaOverAlignedHeader() (string, []string) {
	var source strings.Builder
	source.WriteString(`typedef struct __attribute__((aligned(32))) { int inner_value; double inner_real; } Inner;
typedef struct __attribute__((aligned(16))) { float inner_left; float inner_right; } FloatInner;
typedef int (*Entry)(int);
Inner echo_inner(Inner); FloatInner echo_float_inner(FloatInner);
`)
	functions := []string{"echo_inner", "echo_float_inner"}
	patterns := []struct{ name, attributes, fields string }{
		{"Scalar", "", "signed char tag; int value __attribute__((aligned(16))); double real;"},
		{"Maximum", "", "signed char tag; int value __attribute__((aligned)); double real;"},
		{"Type", "", "signed char tag; _Alignas(Inner) int value; double real;"},
		{"Nested", "", "signed char tag; Inner items[2][2]; double real;"},
		{"FloatNested", "", "FloatInner items[1]; float real;"},
		{"FloatRoot", "__attribute__((aligned(32)))", "float left; float right;"},
		{"Packed", "__attribute__((packed))", "signed char tag; int value __attribute__((aligned(16))); double real;"},
		{"Array", "", "signed char tag; double values[2] __attribute__((aligned(32))); int tail;"},
		{"Slot", "", "signed char tag; Entry entry __attribute__((aligned(16))); double real;"},
		{"Mixed", "", "signed char tag; int value __attribute__((packed,aligned(16))); double real;"},
		{"Natural", "", "signed char tag; double real; int tail;"},
		{"HFA", "", "float left; float right;"},
		{"NaturalPacked", "__attribute__((packed))", "signed char tag; double real; int tail;"},
		{"NaturalNested", "", "signed char tag; FloatInner items[2]; double real;"},
	}
	fields := regexp.MustCompile(`\b(tag|value|real|items|left|right|values|tail|entry)\b`)
	for _, cap := range []int{1, 2, 4, 8, 16} {
		for _, pattern := range patterns {
			// Sweep all caps for representative placement and ABI shapes;
			// the other source-attribute forms share the pack(2) probe.
			if cap != 2 && pattern.name != "Scalar" && pattern.name != "Type" && pattern.name != "Nested" && pattern.name != "Array" && pattern.name != "FloatRoot" && pattern.name != "HFA" {
				continue
			}
			name := fmt.Sprintf("%s%d", pattern.name, cap)
			// Distinct logical names retain the generator's ambiguity guard.
			members := fields.ReplaceAllString(pattern.fields, "${0}"+name)
			fmt.Fprintf(&source, "#pragma pack(push,%d)\ntypedef struct %s {%s} %s;\n#pragma pack(pop)\n%s echo_%s(%s);\n", cap, pattern.attributes, members, name, name, name, name)
			functions = append(functions, "echo_"+name)
		}
	}
	return source.String(), functions
}

func TestPragmaMemberAlignmentAcrossTargets(t *testing.T) {
	source, functions := pragmaOverAlignedHeader()
	path, compiler := headerFile(t, source), compilerTool(t)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: functions})
			if err != nil {
				t.Fatal(err)
			}
			scalar, err := h.LookupRecord("Scalar2")
			if err != nil {
				t.Fatal(err)
			}
			want := uint64(2)
			if target.os == "windows" {
				want = 16
			}
			if scalar.MaxFieldAlignment != 2 || scalar.MemberAlignments[1] != want || scalar.Layout.Offsets[1] != want {
				t.Fatal("producer-specific alignment", scalar)
			}
			nested, _ := h.LookupRecord("Nested2")
			want = 2
			if target.os == "windows" {
				want = 32
			}
			if nested.MemberAlignments[1] != want || nested.Layout.Offsets[1] != want {
				t.Fatal("nested alignment", nested)
			}
			packed, _ := h.LookupRecord("Packed2")
			if target.os != "windows" && packed.MemberAlignments[1] != 2 {
				t.Fatal("packed expression alignment was not reconciled", packed)
			}
			if target.arch == "386" && target.os == "windows" && !scalar.IndirectArgument {
				t.Fatal("MSVC argument lowering missing", scalar)
			}
			scalar.MemberAlignments[1] = 1
			fresh, _ := h.LookupRecord("Scalar2")
			if fresh.MemberAlignments[1] == 1 {
				t.Fatal("member alignment snapshot is shared")
			}
			generated, err := h.CgoSource("bindings", "Bindings")
			if err != nil || !strings.Contains(string(generated), "MemberAlignments: []uint64{") {
				t.Fatal("native metadata persistence", err)
			}
			if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
				t.Fatal("pragma record accepted without a C bridge")
			}
		})
	}
}

func TestPragmaMemberAlignmentBridgeMSVCAndMinGW(t *testing.T) {
	source, functions := pragmaOverAlignedHeader()
	testPackedBitfieldBridgeMSVCAndMinGW(t, source, functions...)
}

func TestMemberStorageAlignmentValidation(t *testing.T) {
	for _, test := range []struct{ expression, base, offset, want uint64 }{
		{16, 2, 2, 2}, {8, 16, 20, 4}, {32, 32, 0, 32}, {4, 2, 0, 2}, {8, 32, 19, 1},
	} {
		got, err := memberStorageAlignment(test.expression, test.base, test.offset)
		if err != nil || got != test.want {
			t.Fatal(test, got, err)
		}
	}
	for _, pair := range [][2]uint64{{0, 2}, {3, 2}, {16, 0}, {16, 3}, {131072, 16}} {
		if _, err := memberStorageAlignment(pair[0], pair[1], 0); err == nil {
			t.Fatal("invalid probe accepted", pair)
		}
	}
	r := Record{Name: "S", Description: abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "tag", Type: abi.TypeDesc{Type: abi.I8}}, {Name: "value", Type: abi.TypeDesc{Type: abi.I32}}}}, MaxFieldAlignment: 2, MemberAlignments: []uint64{1, 16}, Layout: abi.Layout{Size: 32, Alignment: 16, Offsets: []uint64{0, 16}}}
	if err := validRecordLayout(r); err != nil {
		t.Fatal(err)
	}
	for _, alignments := range [][]uint64{nil, {1}, {1, 0}, {1, 3}, {1, 32}, {1, 131072}} {
		invalid := r
		invalid.MemberAlignments = alignments
		if err := validRecordLayout(invalid); err == nil {
			t.Fatal("invalid native metadata accepted", alignments)
		}
	}
	invalid := r
	invalid.Layout.Offsets = []uint64{0, 18}
	if err := validRecordLayout(invalid); err == nil {
		t.Fatal("incompatible native offset accepted")
	}
	invalid = r
	invalid.MaxFieldAlignment = 0
	if err := validRecordLayout(invalid); err == nil {
		t.Fatal("pragma metadata on ordinary record accepted")
	}
	other := r
	other.MemberAlignments = []uint64{1, 2}
	if _, err := newDirectTypes([]Record{r, other}, "Bindings"); err == nil {
		t.Fatal("distinct native member alignment collapsed")
	}
}
