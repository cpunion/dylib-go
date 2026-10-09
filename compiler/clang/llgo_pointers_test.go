package clang

import (
	"context"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestDirectFunctionPointersAcrossTargets(t *testing.T) {
	path := headerFile(t, functionPointerHeader)
	names := []string{"apply", "direct", "decayed", "factory", "anonymous_factory", "pair_apply", "pointer_apply"}
	for _, target := range targets {
		if target.arch == "386" {
			continue
		}
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: names})
			if err != nil {
				t.Fatal(err)
			}
			source, err := h.LLGoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"type BindingsCallback0 func(int32, int32) int32", "Apply(p0 BindingsCallback0", "Direct(p0 BindingsCallback0", "Decayed(p0 BindingsCallback0", "Factory() (result BindingsCallback0", "type BindingsCallback1 func(BindingsRecord0) BindingsRecord0", "type BindingsCallback2 func(*BindingsRecord0)"} {
				if !strings.Contains(string(source), marker) {
					t.Fatalf("missing %s:\n%s", marker, source)
				}
			}
			if strings.Contains(string(source), "type BindingsCallback3") {
				t.Fatal("identical function prototypes were duplicated")
			}
			h.Functions[0].FunctionPointers[0].Signature.Convention = abi.StdCall
			if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
				t.Fatal("unsupported native pointer convention emitted")
			}
		})
	}
}
