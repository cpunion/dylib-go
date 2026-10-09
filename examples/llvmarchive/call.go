//go:build libffi && cgo && (linux || darwin || windows)

package main

import (
	"context"
	"os"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/llvm"
)

func call(input string) (abi.Value, error) {
	archive, err := llvm.CompileArchive(context.Background(), input, llvm.Options{Compiler: os.Getenv("DYLIB_LLC")})
	if err != nil {
		return abi.Value{}, err
	}
	defer archive.Close()
	session := dylib.New(dylib.Options{LibraryPaths: archive.SourceDirectories})
	defer session.Close()
	if err := session.Load(archive.Path); err != nil {
		return abi.Value{}, err
	}
	if err := archive.Close(); err != nil {
		return abi.Value{}, err
	}
	if err := session.Link("add"); err != nil {
		return abi.Value{}, err
	}
	fn, err := session.Bind("add", abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}})
	if err != nil {
		return abi.Value{}, err
	}
	return fn.Call(abi.Int32(20), abi.Int32(22))
}
