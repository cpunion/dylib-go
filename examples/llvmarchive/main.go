//go:build libffi && cgo && (linux || darwin || windows)

package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: llvmarchive <library.a>")
	}
	value, err := call(os.Args[1])
	if err != nil {
		panic(err)
	}
	fmt.Println(int32(value.Bits))
}
