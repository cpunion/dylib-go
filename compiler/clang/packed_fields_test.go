package clang

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/ramblelabs/dylib-go/abi"
)

func TestCompilerPackedMembers(t *testing.T) {
	path := headerFile(t, `typedef struct { signed char tag; double values[2] __attribute__((packed)); short tail; void *context __attribute__((packed)); } Member;
typedef struct { Member items[2][2]; int count; } Outer;
typedef struct { char marker; double samples[2] __attribute__((packed,aligned(4))); int counter; _Bool truth __attribute__((packed)); } Aligned;
Member echo(Member); Outer nest(Outer); Aligned aligned(Aligned);`)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"echo", "nest", "aligned"}})
			if err != nil {
				t.Fatal(err)
			}
			r, err := h.LookupRecord("Member")
			want := abi.Layout{Size: 28, Alignment: 2, Offsets: []uint64{0, 1, 18, 20}}
			if target.arch == "386" {
				want.Size = 24
			}
			if err != nil || r.Packed || !reflect.DeepEqual(r.PackedFields, []int{1, 3}) || !reflect.DeepEqual(r.Layout, want) {
				t.Fatal("packed member layout:", r, err)
			}
			r.PackedFields[0] = 0
			if h.Records[0].PackedFields[0] != 1 {
				t.Fatal("packed member snapshot aliases the header")
			}
			a, err := h.LookupRecord("Aligned")
			if err != nil || a.Layout.Size != 28 || a.Layout.Alignment != 4 || a.Layout.Offsets[1] != 4 || !reflect.DeepEqual(a.FieldAlignments, []FieldAlignment{{Position: 1, Aligned: 4}}) {
				t.Fatal("member packing and alignment:", a, err)
			}
			source, err := h.CgoSource("bindings", "Bindings")
			if err != nil || !strings.Contains(string(source), "PackedFields: []int{1, 3}") || !strings.Contains(string(source), "f1[2] __attribute__((packed)) __attribute__((aligned(4)))") {
				t.Fatal("packed member bridge:", err)
			}
			if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
				t.Fatal("direct llgo accepted member packing")
			}
			if h.Target.CheckHost() == nil {
				if _, err := h.ForHost("echo"); err == nil || !strings.Contains(err.Error(), "compiled C adapter") {
					t.Fatal("dynamic adapter accepted member packing:", err)
				}
			}
		})
	}
}

func TestPackedMemberValidation(t *testing.T) {
	r := Record{Name: "S", Description: abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "a", Type: abi.TypeDesc{Type: abi.I32}}, {Name: "b", Type: abi.TypeDesc{Type: abi.I32}}}}}
	for _, fields := range [][]int{{-1}, {2}, {1, 1}, {1, 0}} {
		r.PackedFields = fields
		if err := validatePackedFields(r); err == nil {
			t.Fatal("invalid member packing accepted:", fields)
		}
	}
	r.PackedFields, r.Bitfields = []int{0}, []Bitfield{{Position: 0, Width: 3}}
	if err := validatePackedFields(r); err != nil {
		t.Fatal("named packed bitfield rejected:", err)
	}
}
