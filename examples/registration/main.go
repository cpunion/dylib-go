//go:build libffi && cgo && (darwin || linux || windows)

// This example groups the leases behind a C-retained record and callback.
package main

import (
	"errors"
	"fmt"
	"os"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
)

func run(path string) (err error) {
	session := dylib.New(dylib.Options{})
	retained := false
	defer func() {
		if !retained {
			session.Close()
		}
	}()
	if err := session.Load(path); err != nil {
		return err
	}
	store, err := session.Bind("registration_store", abi.Signature{Args: []abi.Type{abi.Pointer, abi.Pointer}})
	if err != nil {
		return err
	}
	invoke, err := session.Bind("registration_call", abi.Signature{Result: abi.I32})
	if err != nil {
		return err
	}
	clear, err := session.Bind("registration_clear", abi.Signature{Result: abi.I32})
	if err != nil {
		return err
	}
	description := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{
		{Name: "a", Type: abi.TypeDesc{Type: abi.I32}},
		{Name: "b", Type: abi.TypeDesc{Type: abi.I32}},
	}}
	initial, err := abi.StructValue(description, abi.Int32(20), abi.Int32(22))
	if err != nil {
		return err
	}
	owner, err := abi.NewNativeValue(description, initial)
	if err != nil {
		return err
	}
	defer func() {
		if !retained {
			owner.Close()
		}
	}()
	callback, err := abi.NewCallback(abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}, func(args []abi.Value) (abi.Value, error) {
		return abi.Int32(int32(args[0].Bits) + int32(args[1].Bits)), nil
	})
	if err != nil {
		return err
	}
	defer func() {
		if !retained {
			callback.Close()
		}
	}()
	registration, err := dylib.NewRegistration(dylib.RegistrationResources{
		Functions: []*dylib.Function{store, invoke, clear},
		Values:    []*abi.NativeValue{owner},
		Callbacks: []*abi.Callback{callback},
	}, func(leases dylib.RegistrationLeases) error {
		// This API stops synchronously; threaded APIs must also join here.
		_, err := leases.Functions[2].Call()
		return err
	})
	if err != nil {
		return err
	}
	retained = true
	defer func() {
		cleanup := registration.Close()
		// Failed unregister keeps owners retained. A long-running caller must
		// keep the registration handle and retry before closing its owners.
		retained = cleanup != nil
		err = errors.Join(err, cleanup)
	}()
	if err := registration.WithLeases(func(leases dylib.RegistrationLeases) error {
		data, err := leases.Values[0].Address()
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
		return err
	}
	return registration.WithLeases(func(leases dylib.RegistrationLeases) error {
		value, err := leases.Functions[1].Call()
		if err == nil {
			fmt.Println(int32(value.Bits))
		}
		return err
	})
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: registration <library>")
	}
	if err := run(os.Args[1]); err != nil {
		panic(err)
	}
}
