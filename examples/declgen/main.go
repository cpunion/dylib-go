// This utility emits declarations for the dynamic ABI, complementing static
// llcppg bindings. The library API is compiler/clang.Parse and Header.GoSource.
package main

import (
	"context"
	"flag"
	"os"

	"github.com/cpunion/dylib-go/compiler/clang"
)

func main() {
	compiler := flag.String("clang", "", "Clang executable")
	target := flag.String("target", "", "Clang target triple")
	output := flag.String("out", "", "output Go source path")
	packageName := flag.String("package", "main", "Go package name")
	packing := flag.Bool("pack", false, "generate with Clang -fpack-struct=1")
	direct := flag.Bool("llgo", false, "emit direct fixed-cdecl llgo bindings; -var names the binding type")
	variable := flag.String("var", "Declarations", "Go declaration variable")
	flag.Parse()
	if flag.NArg() < 2 || *output == "" {
		panic("usage: declgen -out=<file.go> [-target=<triple>] <header.h> <function>...")
	}
	var flags []string
	if *packing {
		flags = append(flags, "-fpack-struct=1")
	}
	header, err := clang.Parse(context.Background(), flag.Arg(0), clang.Options{Compiler: *compiler, Target: *target, Flags: flags, Functions: flag.Args()[1:]})
	if err != nil {
		panic(err)
	}
	var source []byte
	if *direct {
		source, err = header.LLGoSource(*packageName, *variable)
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
