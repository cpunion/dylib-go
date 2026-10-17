package abi

import "testing"

func TestVectorcallRequiresCompiledAdapter(t *testing.T) {
	s := Signature{Result: I32, Args: []Type{I32, I32}, Convention: VectorCall}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Prepare(s)
	if err == nil || plan != nil {
		if plan != nil {
			plan.Close()
		}
		t.Fatal("vectorcall must not fall back to a dynamic C ABI", err)
	}
	callback, err := NewCallback(s, func([]Value) (Value, error) { return Int32(42), nil })
	if err == nil || callback != nil {
		if callback != nil {
			callback.Close()
		}
		t.Fatal("vectorcall must not create a default-ABI callback", err)
	}
}
