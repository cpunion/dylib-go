package abi

import (
	"runtime"
	"strings"
	"testing"
)

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

func TestLargeSignatureValidation(t *testing.T) {
	s := Signature{Result: I64, Args: make([]Type, 64)}
	for i := range s.Args {
		s.Args[i] = I32
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	s.Args[63] = Void
	if err := s.Validate(); err == nil {
		t.Fatal("last argument was not validated")
	}
}

func TestThiscallSignatureAndBackend(t *testing.T) {
	for _, s := range []Signature{
		{Convention: ThisCall},
		{Convention: ThisCall, Args: []Type{I32}},
		{Convention: ThisCall, Args: []Type{Pointer}, Variadic: true, FixedArgs: 1},
	} {
		if err := s.Validate(); err == nil {
			t.Fatalf("invalid thiscall signature accepted: %+v", s)
		}
	}
	scalar := Signature{Convention: ThisCall, Result: I32, Args: []Type{Pointer, I32}}
	if err := scalar.Validate(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "386" {
		if p, err := Prepare(scalar); err == nil {
			p.Close()
			t.Fatal("unavailable thiscall accepted")
		}
		if cb, err := NewCallback(scalar, func([]Value) (Value, error) { return Int32(0), nil }); err == nil {
			cb.Close()
			t.Fatal("unavailable thiscall callback accepted")
		}
	}
	record := TypeDesc{Type: Struct, Fields: []Field{{Name: "a", Type: TypeDesc{Type: I64}}, {Name: "b", Type: TypeDesc{Type: I64}}}}
	scalar.Result, scalar.ResultType = Struct, &record
	if err := scalar.Validate(); err != nil {
		t.Fatal("compiled bridge signature rejected:", err)
	}
	if Available() && runtime.GOOS == "windows" && runtime.GOARCH == "386" {
		if p, err := Prepare(scalar); err == nil {
			p.Close()
			t.Fatal("unsafe thiscall record result accepted")
		} else if !strings.Contains(err.Error(), "compiled C bridge") {
			t.Fatal(err)
		}
		if cb, err := NewCallback(scalar, func([]Value) (Value, error) { return Value{}, nil }); err == nil {
			cb.Close()
			t.Fatal("unsafe thiscall record callback result accepted")
		} else if !strings.Contains(err.Error(), "compiled C bridge") {
			t.Fatal(err)
		}
	}
}
