//go:build libffi && cgo && (darwin || linux || windows)

// This example supplies a dynamic scalar signature to the libffi backend.
package main

import (
	"fmt"
	"math"
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
	f, err := s.Bind("mixed", abi.Signature{
		Result: abi.F64,
		Args:   []abi.Type{abi.I32, abi.F64, abi.F32, abi.U64},
	})
	if err != nil {
		return err
	}
	v, err := f.Call(abi.Int32(10), abi.Float64(20.5), abi.Float32(1.5), abi.Uint64(10))
	if err != nil {
		return err
	}
	fmt.Println(math.Float64frombits(v.Bits))
	return s.Close()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mixed-bind OBJECT_OR_LIBRARY")
		os.Exit(1)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
