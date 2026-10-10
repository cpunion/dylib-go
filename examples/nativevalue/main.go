//go:build libffi && cgo && (darwin || linux || windows)

// This example lets C retain a leased record across separate calls.
package main

import (
	"fmt"
	"os"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
)

func run(path string) error {
	session := dylib.New(dylib.Options{})
	defer session.Close()
	if err := session.Load(path); err != nil {
		return err
	}
	store, err := session.Bind("native_value_save_pair", abi.Signature{Args: []abi.Type{abi.Pointer}})
	if err != nil {
		return err
	}
	sum, err := session.Bind("native_value_sum_pair", abi.Signature{Result: abi.I32})
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
	defer owner.Close()
	return owner.WithAddress(func(address uintptr) (err error) {
		if _, err = store.Call(abi.Ptr(address)); err != nil {
			return err
		}
		// Unregister before WithAddress releases its lease, even on failure.
		defer func() {
			_, cleanup := store.Call(abi.Ptr(0))
			if err == nil {
				err = cleanup
			}
		}()
		value, err := sum.Call()
		if err != nil {
			return err
		}
		fmt.Println(int32(value.Bits))
		return nil
	})
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: nativevalue <library>")
	}
	if err := run(os.Args[1]); err != nil {
		panic(err)
	}
}
