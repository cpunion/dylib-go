package dylib

import (
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestNativeStorageLayoutsAgainstCompiler(t *testing.T) {
	padded := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{
		{Name: "tag", Type: abi.TypeDesc{Type: abi.I8}},
		{Name: "value", Type: abi.TypeDesc{Type: abi.F64}},
		{Name: "tail", Type: abi.TypeDesc{Type: abi.I16}},
	}}
	nested := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{
		{Name: "tag", Type: abi.TypeDesc{Type: abi.I8}},
		{Name: "inner", Type: padded},
		{Name: "values", Type: abi.TypeDesc{Type: abi.Array, Len: 3, Elem: &abi.TypeDesc{Type: abi.I32}}},
		{Name: "pointer", Type: abi.TypeDesc{Type: abi.Pointer}},
		{Name: "ready", Type: abi.TypeDesc{Type: abi.Bool}},
	}}
	types := []abi.TypeDesc{}
	for _, typ := range []abi.Type{abi.I8, abi.U8, abi.I16, abi.U16, abi.I32, abi.U32, abi.I64, abi.U64, abi.F32, abi.F64, abi.Bool, abi.Pointer} {
		types = append(types, abi.TypeDesc{Type: typ})
	}
	types = append(types, padded, nested,
		abi.TypeDesc{Type: abi.Array, Len: 2, Elem: &padded},
		abi.TypeDesc{Type: abi.Array, Len: 2, Elem: &abi.TypeDesc{Type: abi.Array, Len: 3, Elem: &abi.TypeDesc{Type: abi.I16}}},
		abi.TypeDesc{Type: abi.Array, Len: 3, Elem: &abi.TypeDesc{Type: abi.Pointer, Elem: &padded}},
		abi.TypeDesc{Type: abi.Pointer, Elem: &padded},
	)
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			session := nativeABILibrary(t, "testdata/layout.c", input)
			size, err := session.Bind("native_layout_size", abi.Signature{Result: abi.U64, Args: []abi.Type{abi.I32}})
			if err != nil {
				t.Fatal(err)
			}
			alignment, err := session.Bind("native_layout_alignment", abi.Signature{Result: abi.U64, Args: []abi.Type{abi.I32}})
			if err != nil {
				t.Fatal(err)
			}
			offset, err := session.Bind("native_layout_offset", abi.Signature{Result: abi.U64, Args: []abi.Type{abi.I32, abi.I32}})
			if err != nil {
				t.Fatal(err)
			}
			for index, description := range types {
				layout, err := abi.LayoutOf(description, abi.CDecl)
				if err != nil {
					t.Fatal(err)
				}
				for _, query := range []struct {
					name string
					fn   *Function
					want uint64
				}{{"size", size, layout.Size}, {"alignment", alignment, layout.Alignment}} {
					got, err := query.fn.Call(abi.Int32(int32(index)))
					if err != nil || got != abi.Uint64(query.want) {
						t.Fatalf("type %d %s: compiler=%+v, %v; backend=%d", index, query.name, got, err, query.want)
					}
				}
				members := len(description.Fields)
				if description.Type == abi.Array {
					members = description.Len
				}
				if len(layout.Offsets) != members {
					t.Fatalf("type %d: offsets count %d, want %d", index, len(layout.Offsets), members)
				}
				for member, want := range layout.Offsets {
					got, err := offset.Call(abi.Int32(int32(index)), abi.Int32(int32(member)))
					if err != nil || got != abi.Uint64(want) {
						t.Fatalf("type %d member %d: compiler=%+v, %v; backend=%d", index, member, got, err, want)
					}
				}
			}
		})
	}
}
