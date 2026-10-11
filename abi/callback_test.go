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
	if _, err := lease.Pointer(); !errors.Is(err, ErrCallbackClosed) {
		t.Fatal(err)
	}
	if _, err := (&CallbackLease{}).Pointer(); !errors.Is(err, ErrCallbackClosed) {
		t.Fatal(err)
	}
	lease.Close()
	if !Available() {
		if _, err := NewCallback(Signature{}, func([]Value) (Value, error) { return Value{}, nil }); !errors.Is(err, ErrUnavailable) {
			t.Fatal(err)
		}
	}
}

func TestCallbackPointerLease(t *testing.T) {
	if !Available() {
		t.Skip("requires the native callback backend")
	}
	callback, err := NewCallback(Signature{Result: I32, Args: []Type{I32}}, func(args []Value) (Value, error) { return args[0], nil })
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	lease, err := callback.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	address, err := lease.Address()
	if err != nil {
		t.Fatal(err)
	}
	pointer, err := lease.Pointer()
	if err != nil || pointer == nil || uintptr(pointer) != address {
		t.Fatal(pointer, address, err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if pointer, err := lease.Pointer(); pointer != nil || !errors.Is(err, ErrCallbackClosed) {
		t.Fatal("released pointer lease", pointer, err)
	}
}
