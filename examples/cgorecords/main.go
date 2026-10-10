//go:build cgo && (linux || darwin || windows)

// This example passes and returns C record values without libffi.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	dylib "github.com/cpunion/dylib-go"
)

func load(input string) (*dylib.Session, error) {
	session := dylib.New(dylib.Options{ProcessSymbols: runtime.GOOS != "windows"})
	if runtime.GOOS == "windows" {
		directory := "System32"
		if runtime.GOARCH == "386" && (os.Getenv("PROCESSOR_ARCHITEW6432") != "" || os.Getenv("PROCESSOR_ARCHITECTURE") == "AMD64") {
			directory = "SysWOW64"
		}
		if err := session.Load(filepath.Join(os.Getenv("SystemRoot"), directory, "msvcrt.dll")); err != nil {
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

func call(input string) (int32, error) {
	session, err := load(input)
	if err != nil {
		return 0, err
	}
	defer session.Close()
	bindings, err := NewBindings(session)
	if err != nil {
		return 0, err
	}
	value, err := bindings.Echo_small(BindingsRecord0{A: 20, B: 22})
	return value.A + value.B, err
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: cgorecords <library>")
	}
	result, err := call(os.Args[1])
	if err != nil {
		panic(err)
	}
	fmt.Println(result)
}
