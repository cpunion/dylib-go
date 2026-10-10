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

func TestDirectVariadicLLGoGenerationAcrossTargets(t *testing.T) {
	path := headerFile(t, "double mixed(float,int,...); void empty(int,...); typedef int (*Variadic)(int,...); Variadic factory(void); int consume(Variadic);")
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"mixed", "empty", "factory", "consume"}})
			if err != nil {
				t.Fatal(err)
			}
			h.Functions[0], err = h.Functions[0].WithTail(abi.I8, abi.U8, abi.I16, abi.U16, abi.F32, abi.Bool, abi.I64, abi.U64, abi.Pointer)
			if err != nil {
				t.Fatal(err)
			}
			// Different concrete call tails still share one native pointer prototype.
			for i, index := range []int{2, 3} {
				h.Functions[index].FunctionPointers[0], err = h.Functions[index].FunctionPointers[0].WithTail([]abi.Type{abi.I8, abi.F32}[:i+1]...)
				if err != nil {
					t.Fatal(err)
				}
			}
			source, err := h.LLGoSource("bindings", "Bindings")
			if target.arch == "386" {
				if err == nil {
					t.Fatal("unqualified variadic 386 adapter emitted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{
				"type _BindingsFunction0 func(p0 float32, p1 int32, __llgo_va_list ...any) float64",
				"function(p0, p1, int32(p2), int32(p3), int32(p4), int32(p5), float64(p6), v7, p8, p9, p10)",
				"var v7 int32", "if p7", "v7 = 1",
				"type _BindingsFunction1 func(p0 int32, __llgo_va_list ...any)", "function(p0)",
				"type BindingsCallback0 func(p0 int32, __llgo_va_list ...any) int32",
				"Consume(p0 BindingsCallback0)",
			} {
				if !strings.Contains(string(source), marker) {
					t.Fatalf("missing %s:\n%s", marker, source)
				}
			}
			if strings.Contains(string(source), "type BindingsCallback1") {
				t.Fatal("concrete tails created duplicate native pointer types")
			}
		})
	}
}
