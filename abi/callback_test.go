package abi

import (
	"errors"
	"testing"
)

func TestCallbackValidationAndUnavailable(t *testing.T) {
	for _, sig := range []Signature{
		{Result: Type(255)},
		{Args: []Type{Void}},
		{Args: []Type{I32}, Variadic: true, FixedArgs: 1},
	} {
		if cb, err := NewCallback(sig, func([]Value) (Value, error) { return Value{}, nil }); err == nil {
			cb.Close()
			t.Fatal("invalid callback accepted")
		}
	}
	if _, err := NewCallback(Signature{}, nil); err == nil {
		t.Fatal("nil handler accepted")
	}
	var cb *Callback
	if _, err := cb.Acquire(); !errors.Is(err, ErrCallbackClosed) {
		t.Fatal(err)
	}
	if err := cb.Err(); !errors.Is(err, ErrCallbackClosed) {
		t.Fatal(err)
	}
	cb.Close()
	zero := &Callback{}
	if _, err := zero.Acquire(); !errors.Is(err, ErrCallbackClosed) {
		t.Fatal(err)
	}
	if err := zero.Err(); !errors.Is(err, ErrCallbackClosed) {
		t.Fatal(err)
	}
	zero.Close()
	var lease *CallbackLease
	if _, err := lease.Address(); !errors.Is(err, ErrCallbackClosed) {
		t.Fatal(err)
	}
	lease.Close()
	if !Available() {
		if _, err := NewCallback(Signature{}, func([]Value) (Value, error) { return Value{}, nil }); !errors.Is(err, ErrUnavailable) {
			t.Fatal(err)
		}
	}
}
