package clang

import (
	"context"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestDirectLLGoGenerationAcrossTargets(t *testing.T) {
	path := headerFile(t, "int add(int,int); _Bool truth(_Bool); void *pointer(void *); void zero(void);")
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"add", "truth", "pointer", "zero"}})
			if err != nil {
				t.Fatal(err)
			}
			source, err := h.LLGoSource("bindings", "Bindings")
			if target.arch == "386" {
				if err == nil {
					t.Fatal("unqualified 386 adapter emitted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.ParseComments|parser.AllErrors); err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"//go:build llgo && cgo", "//llgo:type C", "NewBindings", "WithAddress", "func (b *Bindings) Zero() error", "func (b *Bindings) Truth(p0 bool)", "unsafe.Pointer"} {
				if !strings.Contains(string(source), marker) {
					t.Fatalf("missing %s:\n%s", marker, source)
				}
			}
		})
	}
}

func TestDirectLLGoRejectsUnsupportedDeclarations(t *testing.T) {
	compiler := compilerTool(t)
	for _, source := range []string{
		"int f(int,...);",
		"void f(int (*)(int));",
	} {
		h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: compiler, Functions: []string{"f"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
			t.Fatal("unsupported direct adapter emitted:", source)
		}
	}
	h, err := Parse(context.Background(), headerFile(t, "int f(int);"), Options{Compiler: compiler, Target: "x86_64-unknown-linux-gnu", Functions: []string{"f"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "_", "type", "abi", "clang", "dylib", "unsafe", "fmt", "int32", "bool", "error", "nil"} {
		if _, err := h.LLGoSource("bindings", name); err == nil {
			t.Fatal("invalid binding type accepted:", name)
		}
	}
	h.Functions[0].Signature.Convention = abi.StdCall
	if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
		t.Fatal("unsupported convention emitted")
	}
	h.Functions[0].Signature.Convention = abi.Default
	if _, err := h.LLGoSource("bindings", "Bindings"); err != nil {
		t.Fatal("default C convention rejected:", err)
	}
	h.Functions = append(h.Functions, Declaration{Name: "F", Symbol: "F", Signature: h.Functions[0].Signature})
	if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
		t.Fatal("ambiguous method name emitted")
	}
}
