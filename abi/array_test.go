package abi

import (
	"reflect"
	"testing"
)

func TestArrayValues(t *testing.T) {
	elem := TypeDesc{Type: I32}
	d := TypeDesc{Type: Array, Len: 2, Elem: &elem}
	v, err := ArrayValue(d, Int32(20), Int32(22))
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Validate(d); err != nil {
		t.Fatal(err)
	}
	zero := Zero(d)
	if !reflect.DeepEqual(zero.Aggregate.Fields, []Value{Int32(0), Int32(0)}) {
		t.Fatal(zero)
	}
	clone := d.Clone()
	clone.Len, clone.Elem.Type = 3, U32
	if d.Len != 2 || d.Elem.Type != I32 || v.Description().Elem.Type != I32 {
		t.Fatal("array descriptors share mutable state")
	}
	if _, err := ArrayValue(d, Int32(20)); err == nil {
		t.Fatal("accepted too few array elements")
	}
	if _, err := ArrayValue(d, Int32(20), Uint32(22)); err == nil {
		t.Fatal("accepted wrong element type")
	}
	for _, wrong := range []TypeDesc{
		{Type: Array, Len: 1, Elem: &elem},
		{Type: Array, Len: 2, Elem: &TypeDesc{Type: U32}},
	} {
		if err := v.Validate(wrong); err == nil {
			t.Fatal("accepted mismatched array layout")
		}
	}
	ptr := TypeDesc{Type: Pointer, Elem: &d}
	if err := AddressOf(&v).Validate(ptr); err != nil {
		t.Fatal(err)
	}
	for _, s := range []Signature{
		{Args: []Type{Array}, ArgTypes: []TypeDesc{d}},
		{Result: Array, ResultType: &d},
		{Args: []Type{I32, Array}, ArgTypes: []TypeDesc{elem, d}, Variadic: true, FixedArgs: 1},
	} {
		if s.Validate() == nil {
			t.Fatal("accepted an array directly in a C signature")
		}
	}
	if err := (Signature{Args: []Type{Pointer}, ArgTypes: []TypeDesc{ptr}}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestArrayDescriptorLimits(t *testing.T) {
	elem := TypeDesc{Type: U8}
	large := TypeDesc{Type: Array, Len: 65536, Elem: &elem}
	if err := large.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, d := range []TypeDesc{
		{Type: Array, Len: 2}, {Type: Array, Elem: &elem},
		{Type: Array, Len: -1, Elem: &elem},
		{Type: Array, Len: 65537, Elem: &elem},
		{Type: Array, Len: 1, Elem: &TypeDesc{Type: Void}},
		{Type: I32, Len: 2}, {Type: Pointer, Elem: &elem, Len: 2},
		{Type: Array, Len: 2, Elem: &large},
		{Type: Struct, Fields: []Field{{Name: "a", Type: large}, {Name: "b", Type: elem}}},
	} {
		if err := d.Validate(); err == nil {
			t.Fatalf("accepted invalid array descriptor %+v", d)
		}
	}
	cycle := TypeDesc{Type: Array, Len: 1}
	cycle.Elem = &cycle
	if err := cycle.Validate(); err == nil {
		t.Fatal("accepted cyclic descriptor")
	}
	malformed := Value{Type: Array, Aggregate: &Aggregate{Type: cycle, Fields: []Value{Uint8(0)}}}
	if err := malformed.Validate(TypeDesc{Type: Array, Len: 1, Elem: &elem}); err == nil {
		t.Fatal("accepted a value with a cyclic descriptor")
	}
}
