//go:build libffi && cgo

package abi

import (
	"strings"
	"testing"
)

func TestArrayNativeSizeLimit(t *testing.T) {
	elem := TypeDesc{Type: U64}
	array := TypeDesc{Type: Array, Elem: &elem, Len: 10000}
	record := TypeDesc{Type: Struct, Fields: []Field{{Name: "values", Type: array}}}
	// This descriptor is within the logical element budget, but its C layout
	// exceeds the byte budget. Preparation must reject it without a native call.
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Prepare(Signature{Args: []Type{Struct}, ArgTypes: []TypeDesc{record}})
	if err == nil {
		plan.Close()
		t.Fatal("accepted native storage larger than 64 KiB")
	}
	if !strings.Contains(err.Error(), "64 KiB") {
		t.Fatal(err)
	}
}
