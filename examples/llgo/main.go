//go:build llgo && cgo && (darwin || linux || windows)

// This example uses a caller-defined C signature without the libffi backend.
package main

import (
	"fmt"
	"os"
	"unsafe"

	dylib "github.com/cpunion/llgo-dylib"
)

// llgo emits the indirect call using the native C calling convention.
//
//llgo:type C
type mixedFunc func(int32, float64, float32, uint64) float64

func run(path string) error {
	s := dylib.New(dylib.Options{})
	defer s.Close()
	if err := s.Load(path); err != nil {
		return err
	}
	symbol, err := s.Resolve("mixed")
	if err != nil {
		return err
	}
	var result float64
	err = symbol.WithAddress(func(address uintptr) error {
		function := *(*mixedFunc)(unsafe.Pointer(&address))
		result = function(10, 20.5, 1.5, 10)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Println(result)
	return s.Close()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mixed-llgo OBJECT_OR_LIBRARY")
		os.Exit(1)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
