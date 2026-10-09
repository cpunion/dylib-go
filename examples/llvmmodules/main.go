//go:build libffi && cgo && (linux || darwin || windows)

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/llvm"
)

func call(inputs []string) (abi.Value, error) {
	compiler, linker := os.Getenv("DYLIB_LLC"), os.Getenv("DYLIB_LLVM_LINK")
	if linker == "" && compiler != "" {
		linker = filepath.Join(filepath.Dir(compiler), "llvm-link")
	}
	object, err := llvm.CompileModules(context.Background(), inputs, llvm.MergeOptions{
		Options: llvm.Options{Compiler: compiler}, Linker: linker,
	})
	if err != nil {
		return abi.Value{}, err
	}
	defer object.Close()
	session := dylib.New(dylib.Options{LibraryPaths: object.SourceDirectories})
	defer session.Close()
	if err := session.Load(object.Path); err != nil {
		return abi.Value{}, err
	}
	if err := object.Close(); err != nil {
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

func main() {
	if len(os.Args) < 2 {
		panic("usage: llvmmodules <module.ll|module.bc> ...")
	}
	value, err := call(os.Args[1:])
	if err != nil {
		panic(err)
	}
	fmt.Println(int32(value.Bits))
}
