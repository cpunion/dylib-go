//go:build libffi && cgo && (darwin || linux || windows)

// This example passes a Go closure through a dynamically described C entry.
package main

import (
	"fmt"
	"os"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
)

func run(path string) error {
	s := dylib.New(dylib.Options{})
	defer s.Close()
	if err := s.Load(path); err != nil {
		return err
	}
	f, err := s.Bind("callback_add", abi.Signature{
		Result: abi.I32,
		Args:   []abi.Type{abi.Pointer, abi.I32, abi.I32},
	})
	if err != nil {
		return err
	}
	offset := int32(2)
	callback, err := abi.NewCallback(abi.Signature{
		Result: abi.I32,
		Args:   []abi.Type{abi.I32, abi.I32},
	}, func(args []abi.Value) (abi.Value, error) {
		return abi.Int32(int32(args[0].Bits) + int32(args[1].Bits) + offset), nil
	})
	if err != nil {
		return err
	}
	defer callback.Close()
	err = callback.WithAddress(func(address uintptr) error {
		value, err := f.Call(abi.Ptr(address), abi.Int32(20), abi.Int32(20))
		if err == nil {
			fmt.Println(int32(value.Bits))
		}
		return err
	})
	if err != nil {
		return err
	}
	if err := callback.Err(); err != nil {
		return err
	}
	return s.Close()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: callback OBJECT_OR_LIBRARY")
		os.Exit(1)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
