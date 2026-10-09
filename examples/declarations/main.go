//go:build libffi && cgo && (linux || darwin || windows)

package main

import (
	"fmt"
	"os"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
)

func call(input string) (abi.Value, error) {
	declaration, err := Declarations.ForHost("callback_pair")
	if err != nil {
		return abi.Value{}, err
	}
	session := dylib.New(dylib.Options{})
	defer session.Close()
	if err := session.Load(input); err != nil {
		return abi.Value{}, err
	}
	if err := session.Link(declaration.Symbol); err != nil {
		return abi.Value{}, err
	}
	fn, err := session.Bind(declaration.Symbol, declaration.Signature)
	if err != nil {
		return abi.Value{}, err
	}
	record, err := Declarations.LookupRecord("Pair")
	if err != nil {
		return abi.Value{}, err
	}
	argument, err := abi.StructValue(record.Description, abi.Int32(20), abi.Int32(22))
	if err != nil {
		return abi.Value{}, err
	}
	pointer, err := declaration.LookupFunctionPointer(0)
	if err != nil {
		return abi.Value{}, err
	}
	callback, err := abi.NewCallback(pointer.Signature, func(args []abi.Value) (abi.Value, error) { return args[0], nil })
	if err != nil {
		return abi.Value{}, err
	}
	defer callback.Close()
	var result abi.Value
	err = callback.WithAddress(func(address uintptr) error {
		var err error
		result, err = fn.Call(abi.Ptr(address), argument)
		return err
	})
	if err == nil {
		err = callback.Err()
	}
	return result, err
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: declarations <library>")
	}
	value, err := call(os.Args[1])
	if err != nil {
		panic(err)
	}
	fmt.Println(int32(value.Bits))
}
