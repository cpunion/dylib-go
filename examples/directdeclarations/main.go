//go:build llgo && cgo && (linux || darwin || windows)

// This example calls a generated, statically typed C adapter without libffi.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	dylib "github.com/cpunion/dylib-go"
)

func call(input string) (float64, error) {
	session, err := load(input)
	if err != nil {
		return 0, err
	}
	defer session.Close()
	bindings, err := NewBindings(session)
	if err != nil {
		return 0, err
	}
	value, err := bindings.Echo_small(BindingsRecord3{A: 20, B: 22})
	return float64(value.A + value.B), err
}

func load(input string) (*dylib.Session, error) {
	session := dylib.New(dylib.Options{ProcessSymbols: runtime.GOOS != "windows"})
	if runtime.GOOS == "windows" {
		if err := session.Load(filepath.Join(os.Getenv("SystemRoot"), "System32", "msvcrt.dll")); err != nil {
			session.Close()
			return nil, err
		}
	}
	if err := session.Load(input); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: directdeclarations <library>")
	}
	result, err := call(os.Args[1])
	if err != nil {
		panic(err)
	}
	fmt.Println(result)
}
