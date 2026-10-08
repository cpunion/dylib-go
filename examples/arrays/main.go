//go:build libffi && cgo && (darwin || linux || windows)

// This example describes C array storage without exposing Go memory to C.
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
	array := abi.TypeDesc{Type: abi.Array, Len: 2, Elem: &abi.TypeDesc{Type: abi.I32}}
	record := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "values", Type: array}}}
	values, err := abi.ArrayValue(array, abi.Int32(20), abi.Int32(22))
	if err != nil {
		return err
	}
	arg, err := abi.StructValue(record, values)
	if err != nil {
		return err
	}
	f, err := s.Bind("sum_array_i32", abi.Signature{
		Result: abi.F64, Args: []abi.Type{abi.Struct}, ArgTypes: []abi.TypeDesc{record},
	})
	if err != nil {
		return err
	}
	v, err := f.Call(arg)
	if err != nil {
		return err
	}
	fmt.Println(math.Float64frombits(v.Bits))
	return s.Close()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: arrays LIBRARY")
		os.Exit(1)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
