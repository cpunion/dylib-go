package abi

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRecordIdentityMetadata(t *testing.T) {
	a := TypeDesc{Type: Struct, RecordID: "library:A", Fields: []Field{{Name: "value", Type: TypeDesc{Type: I32}}}}
	b := a.Clone()
	b.RecordID = "library:B"
	if err := a.Validate(); err != nil || a.RecordID != "library:A" {
		t.Fatal("invalid logical record identity", err)
	}
	value, err := StructValue(a, Int32(42))
	if err != nil || value.Validate(b) != nil || value.Description().RecordID != a.RecordID {
		t.Fatal("identity changed physical value compatibility", err)
	}
	encoded, err := json.Marshal(a)
	var decoded TypeDesc
	if err != nil || json.Unmarshal(encoded, &decoded) != nil || !reflect.DeepEqual(a, decoded) {
		t.Fatal("record identity JSON round trip failed", err)
	}
	plain := a.Clone()
	plain.RecordID = ""
	encoded, _ = json.Marshal(plain)
	if strings.Contains(string(encoded), "RecordID") {
		t.Fatal("empty identity changed legacy JSON")
	}
	for _, bad := range []TypeDesc{{Type: I32, RecordID: "scalar"}, {Type: Pointer, RecordID: "pointer"}, {Type: Struct, RecordID: strings.Repeat("x", 4097), Fields: a.Fields}} {
		if bad.Validate() == nil {
			t.Fatal("invalid record identity accepted")
		}
	}
}

func TestRecordIdentityABIShape(t *testing.T) {
	child := TypeDesc{Type: Struct, RecordID: "Child", Fields: []Field{{Name: "value", Type: TypeDesc{Type: I32}}}}
	array := TypeDesc{Type: Array, Len: 2, Elem: &child}
	outer := TypeDesc{Type: Struct, RecordID: "Outer", Fields: []Field{{Name: "items", Type: array}}}
	a := Signature{Result: Struct, ResultType: &outer, Args: []Type{Struct}, ArgTypes: []TypeDesc{outer}}
	b := a.Clone()
	b.ResultType.RecordID = "OtherOuter"
	b.ArgTypes[0].Fields[0].Type.Elem.RecordID = "OtherChild"
	if a.ReturnType().RecordID != "Outer" || a.ArgumentType(0).Fields[0].Type.Elem.RecordID != "Child" {
		t.Fatal("identity clone aliases original metadata")
	}
	first, err := a.ABIShape()
	second, otherErr := b.ABIShape()
	if err != nil || otherErr != nil || !reflect.DeepEqual(first, second) || first.ReturnType().RecordID != "" || first.ArgumentType(0).Fields[0].Type.Elem.RecordID != "" {
		t.Fatal("nominal identity changed native ABI shape", err, otherErr)
	}
}
