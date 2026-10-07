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

func TestVariadicValidation(t *testing.T) {
	for _, s := range []Signature{
		{Convention: 99}, {FixedArgs: 1}, {Variadic: true},
		{Variadic: true, FixedArgs: 2, Args: []Type{I32}},
		{Variadic: true, FixedArgs: 1, Convention: StdCall, Args: []Type{I32}},
	} {
		if err := s.Validate(); err == nil {
			t.Fatalf("accepted %+v", s)
		}
	}
	if err := (Signature{Variadic: true, FixedArgs: 1, Args: []Type{I32, F32, U8}}).Validate(); err != nil {
		t.Fatal(err)
	}
}
