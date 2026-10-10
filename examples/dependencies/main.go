//go:build libffi && cgo && (darwin || linux || windows)

// This example imports retained functions and data from another session.
package main

import (
	"fmt"
	"os"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
)

func run(providerPath, consumerPath string) error {
	provider := dylib.New(dylib.Options{})
	defer provider.Close()
	consumer := dylib.New(dylib.Options{})
	defer consumer.Close() // Release imported code before closing its provider.
	if err := provider.Load(providerPath); err != nil {
		return err
	}
	for _, name := range []string{"dependency_add", "dependency_bias"} {
		symbol, err := provider.Resolve(name)
		if err != nil {
			return err
		}
		if err := consumer.DefineSymbol(name, symbol); err != nil {
			return err
		}
	}
	if err := consumer.Load(consumerPath); err != nil {
		return err
	}
	add, err := consumer.Bind("imported_add", abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}})
	if err != nil {
		return err
	}
	value, err := add.Call(abi.Int32(20), abi.Int32(20))
	if err != nil {
		return err
	}
	fmt.Println(int32(value.Bits))
	return consumer.Close()
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: dependencies PROVIDER CONSUMER")
		os.Exit(1)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
