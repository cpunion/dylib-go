// This utility emits declarations for the dynamic ABI, complementing static
// llcppg bindings. The library API is compiler/clang.Parse and Header.GoSource.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/cpunion/dylib-go/abi/signature"
	"github.com/cpunion/dylib-go/compiler/clang"
)

func main() {
	compiler := flag.String("clang", "", "Clang executable")
	target := flag.String("target", "", "Clang target triple")
	output := flag.String("out", "", "output Go source path")
	packageName := flag.String("package", "main", "Go package name")
	packing := flag.Bool("pack", false, "generate with Clang -fpack-struct=1")
	direct := flag.Bool("llgo", false, "emit direct fixed-cdecl llgo bindings; -var names the binding type")
	cgo := flag.Bool("cgo", false, "emit typed cdecl cgo bridges for Go/llgo; -var names the binding type")
	var tails []string
	flag.Func("tail", "concrete variadic name=int8,float32,... (repeat for each function)", func(value string) error {
		tails = append(tails, value)
		return nil
	})
	variable := flag.String("var", "Declarations", "Go declaration variable")
	flag.Parse()
	if flag.NArg() < 2 || *output == "" {
		panic("usage: declgen -out=<file.go> [-target=<triple>] <header.h> <function>...")
	}
	if *direct && *cgo {
		panic("select either -llgo or -cgo")
	}
	var flags []string
	if *packing {
		flags = append(flags, "-fpack-struct=1")
	}
	header, err := clang.Parse(context.Background(), flag.Arg(0), clang.Options{Compiler: *compiler, Target: *target, Flags: flags, Functions: flag.Args()[1:]})
	if err != nil {
		panic(err)
	}
	seen := make(map[string]bool)
	for _, tail := range tails {
		name, types, ok := strings.Cut(tail, "=")
		if !ok || seen[name] {
			panic("expected a unique variadic name=types")
		}
		seen[name] = true
		declaration, err := header.Lookup(name)
		if err != nil {
			panic(err)
		}
		parsed, err := signature.Parse("func tail(" + types + ")")
		if err != nil || len(parsed.Signature.ArgTypes) != 0 {
			panic(fmt.Sprintf("invalid scalar/opaque-pointer variadic tail %q: %v", tail, err))
		}
		declaration, err = declaration.WithTail(parsed.Signature.Args...)
		if err != nil {
			panic(err)
		}
		for i := range header.Functions {
			if header.Functions[i].Name == name {
				header.Functions[i] = declaration
			}
		}
	}
	var source []byte
	if *direct {
		source, err = header.LLGoSource(*packageName, *variable)
	} else if *cgo {
		source, err = header.CgoSource(*packageName, *variable)
	} else {
		source, err = header.GoSource(*packageName, *variable)
	}
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(*output, source, 0600); err != nil {
		panic(err)
	}
}
