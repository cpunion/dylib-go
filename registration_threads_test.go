package dylib

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cpunion/dylib-go/abi"
)

func TestRegistrationJoinsRetainedForeignThreadBeforeReleasingOwners(t *testing.T) {
	session := callbackThreadLibrary(t, "testdata/registration_threads.c")
	start, err := session.Bind("registration_start", abi.Signature{Result: abi.I32, Args: []abi.Type{abi.Pointer, abi.Pointer}})
	if err != nil {
		t.Fatal(err)
	}
	join, err := session.Bind("registration_join", abi.Signature{Result: abi.I32})
	if err != nil {
		t.Fatal(err)
	}
	owner := registrationPair(t)
	entered, release, stopping := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce, stoppingOnce sync.Once
	var borrowed *abi.NativeValueLease
	callback, err := abi.NewCallback(abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}, func(args []abi.Value) (abi.Value, error) {
		close(entered)
		<-release
		value, err := borrowed.Read()
		if err != nil {
			return abi.Value{}, err
		}
		if value.Aggregate.Fields[0] != args[0] || value.Aggregate.Fields[1] != args[1] {
			return abi.Value{}, errors.New("foreign thread lost native storage")
		}
		return abi.Int32(int32(args[0].Bits) + int32(args[1].Bits)), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { callback.Close() })
	r, err := NewRegistration(RegistrationResources{
		Functions: []*Function{start, join}, Values: []*abi.NativeValue{owner}, Callbacks: []*abi.Callback{callback},
	}, func(leases RegistrationLeases) error {
		stoppingOnce.Do(func() { close(stopping) })
		got, err := leases.Functions[1].Call()
		if err == nil && got != abi.Int32(42) {
			err = errors.New("foreign join did not receive callback result")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }); r.Close() })
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
		got, err := leases.Functions[0].Call(abi.Ptr(data), abi.Ptr(entry))
		if err == nil && got != abi.Int32(0) {
			err = errors.New("native thread start failed")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("native thread did not enter callback")
	}
	closed := make(chan error, 4)
	go func() { closed <- session.Close() }()
	go func() { closed <- owner.Close() }()
	go func() { closed <- callback.Close() }()
	waitSessionRetired(t, session)
	waitNativeValueRetired(t, owner)
	go func() { closed <- r.Close() }()
	<-stopping
	select {
	case <-closed:
		t.Fatal("registration or owner released during a retained native callback")
	default:
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < 4; i++ {
		select {
		case err := <-closed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("native join/owner close did not finish")
		}
	}
	if callback.Err() != nil {
		t.Fatal(callback.Err())
	}
}
