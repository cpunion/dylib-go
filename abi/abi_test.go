package abi

import "testing"

func TestSignatureValidation(t *testing.T) {
	for _, s := range []Signature{{Result: 99}, {Args: []Type{Void}}, {Args: make([]Type, 33)}} {
		if s.Validate() == nil {
			t.Fatalf("accepted %+v", s)
		}
	}
	if _, e := Call(0, Signature{}); e == nil {
		t.Fatal("null call accepted")
	}
	if _, e := Call(1, Signature{Result: I32, Args: []Type{I32}}, Float64(2)); e == nil {
		t.Fatal("type mismatch accepted")
	}
}
