package dylib

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/cpunion/dylib-go/abi"
)

func registrationPair(t *testing.T) *abi.NativeValue {
	t.Helper()
	desc := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{
		{Name: "a", Type: abi.TypeDesc{Type: abi.I32}},
		{Name: "b", Type: abi.TypeDesc{Type: abi.I32}},
	}}
	initial, err := abi.StructValue(desc, abi.Int32(20), abi.Int32(22))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := abi.NewNativeValue(desc, initial)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Close() })
	return owner
}

func waitNativeValueRetired(t *testing.T, owner *abi.NativeValue) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := owner.Description(); errors.Is(err, abi.ErrValueClosed) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("native value did not retire")
		}
		runtime.Gosched()
	}
}

func TestRegistrationRetainsCodeValueAndCallbackDuringRetirement(t *testing.T) {
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			session := nativeABILibrary(t, "examples/registration/testdata/exports.c", input)
			bind := func(name string, result abi.Type, args ...abi.Type) *Function {
				t.Helper()
				f, err := session.Bind(name, abi.Signature{Result: result, Args: args})
				if err != nil {
					t.Fatal(err)
				}
				return f
			}
			store := bind("registration_store", abi.Void, abi.Pointer, abi.Pointer)
			invoke := bind("registration_call", abi.I32)
			clear := bind("registration_clear", abi.I32)
			owner := registrationPair(t)
			var borrowed *abi.NativeValueLease
			callback, err := abi.NewCallback(abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}, func(args []abi.Value) (abi.Value, error) {
				value, err := borrowed.Read()
				if err != nil {
					return abi.Value{}, err
				}
				if value.Aggregate.Fields[0] != args[0] || value.Aggregate.Fields[1] != args[1] {
					return abi.Value{}, errors.New("native retained value differs from leased snapshot")
				}
				return abi.Int32(int32(args[0].Bits) + int32(args[1].Bits)), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { callback.Close() })
			stopFailure := errors.New("retry unregister")
			stops := 0
			r, err := NewRegistration(RegistrationResources{
				Functions: []*Function{store, invoke, clear}, Values: []*abi.NativeValue{owner}, Callbacks: []*abi.Callback{callback},
			}, func(leases RegistrationLeases) error {
				stops++
				if stops == 1 {
					return stopFailure
				}
				// Native clear calls the retained entry before removing it. The
				// callback reads leased storage while all three owners retire.
				got, err := leases.Functions[2].Call()
				if err == nil && got != abi.Int32(42) {
					err = errors.New("unregister lost retained code/data/callback")
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { r.Close(); r.Close() })
			if err := r.WithLeases(func(leases RegistrationLeases) error {
				borrowed = leases.Values[0]
				data, err := borrowed.Address()
				if err != nil {
					return err
				}
				entry, err := leases.Callbacks[0].Address()
				if err != nil {
					return err
				}
				_, err = leases.Functions[0].Call(abi.Ptr(data), abi.Ptr(entry))
				return err
			}); err != nil {
				t.Fatal(err)
			}
			runtime.GC()
			if err := r.WithLeases(func(leases RegistrationLeases) error {
				got, err := leases.Functions[1].Call()
				if got != abi.Int32(42) {
					t.Error("separate retained call:", got)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			closed := make(chan error, 3)
			go func() { closed <- session.Close() }()
			go func() { closed <- owner.Close() }()
			go func() { closed <- callback.Close() }()
			waitSessionRetired(t, session)
			waitNativeValueRetired(t, owner)
			if _, err := clear.Call(); !errors.Is(err, ErrClosed) {
				t.Fatal("ordinary retired function call:", err)
			}
			if err := r.Close(); !errors.Is(err, stopFailure) {
				t.Fatal("stop failure:", err)
			}
			select {
			case <-closed:
				t.Fatal("an owner released resources while still registered")
			default:
			}
			if err := r.Close(); err != nil || callback.Err() != nil {
				t.Fatal("unregister retry:", err, callback.Err())
			}
			for i := 0; i < 3; i++ {
				select {
				case err := <-closed:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("owner close did not finish after successful unregister")
				}
			}
		})
	}
}

func TestFunctionLeaseReentersDuringSessionRetirement(t *testing.T) {
	session := callbackLibrary(t, "object")
	f, err := session.Bind("invoke_i32", callbackSignature(t, "func invoke_i32(unsafe.Pointer,int32,int32)int32"))
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	var address uintptr
	callback, err := abi.NewCallback(callbackSignature(t, "func cb(int32,int32)int32"), func(args []abi.Value) (abi.Value, error) {
		a, b := int32(args[0].Bits), int32(args[1].Bits)
		if a < 2 {
			return lease.Call(abi.Ptr(address), abi.Int32(a+1), abi.Int32(b-1))
		}
		return abi.Int32(a + b), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	cbLease, err := callback.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer cbLease.Close()
	address, err = cbLease.Address()
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- session.Close() }()
	waitSessionRetired(t, session)
	got, err := lease.Call(abi.Ptr(address), abi.Int32(0), abi.Int32(42))
	if err != nil || got != abi.Int32(42) || callback.Err() != nil {
		t.Fatal("retained nested calls:", got, err, callback.Err())
	}
	lease.Close()
	if _, err := lease.Call(); !errors.Is(err, ErrClosed) {
		t.Fatal("released function lease:", err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
}
