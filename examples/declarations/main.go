//go:build libffi && cgo && (linux || darwin || windows)

package main

import (
	"fmt"
	"os"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
)

func call(input string) (abi.Value, error) {
	declaration, err := Declarations.ForHost("add")
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
	return fn.Call(abi.Int32(20), abi.Int32(22))
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
