//go:build cgo && (linux || darwin || windows)

// This example calls a generated variadic C bridge without libffi.
package main

import (
	"fmt"
	"os"

	dylib "github.com/cpunion/dylib-go"
)

func call(input string) (int32, error) {
	session := dylib.New(dylib.Options{})
	defer session.Close()
	if err := session.Load(input); err != nil {
		return 0, err
	}
	bindings, err := NewBindings(session)
	if err != nil {
		return 0, err
	}
	return bindings.Variable(20, 10, 12)
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: cgodeclarations <library>")
	}
	result, err := call(os.Args[1])
	if err != nil {
		panic(err)
	}
	fmt.Println(result)
}
