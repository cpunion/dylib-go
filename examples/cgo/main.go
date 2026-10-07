//go:build cgo && (darwin || linux || windows)

// This example implements a caller-defined signature with an ordinary cgo
// adapter. The loader itself does not know the signature.
package main

/*
#include <stdint.h>
static double call_mixed(uintptr_t address, int32_t a, double b, float c, uint64_t d) {
    return ((double (*)(int32_t, double, float, uint64_t))address)(a, b, c, d);
}
*/
import "C"

import (
	"fmt"
	"os"

	dylib "github.com/cpunion/llgo-dylib"
)

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
		result = float64(C.call_mixed(C.uintptr_t(address), 10, 20.5, 1.5, 10))
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
		fmt.Fprintln(os.Stderr, "usage: mixed-cgo OBJECT_OR_LIBRARY")
		os.Exit(1)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
