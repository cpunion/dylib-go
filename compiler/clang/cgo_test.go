package clang

import (
	"context"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestCgoGenerationAcrossTargets(t *testing.T) {
	path := headerFile(t, "int add(int,int); _Bool truth(_Bool); void *pointer(void *); void zero(void); int variable(int,...); typedef int (*Entry)(int,int); Entry factory(void); int apply(Entry,int,int); int forward(int (*)(int,...),int);")
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"add", "truth", "pointer", "zero", "variable", "factory", "apply", "forward"}})
			if err != nil {
				t.Fatal(err)
			}
			h.Functions[4], err = h.Functions[4].WithTail(abi.I8, abi.F32)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"Bindings", "绑定"} {
				source, err := h.CgoSource("bindings", name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.ParseComments|parser.AllErrors); err != nil {
					t.Fatal(err)
				}
				for _, marker := range []string{"//go:build cgo", "import \"C\"", "(int32_t,...)", "int32_t p1,float p2", "(int8_t)p1", "WithAddress", "unsafe.Pointer", "Target.CheckHost()"} {
					if !strings.Contains(string(source), marker) {
						t.Fatalf("missing %s:\n%s", marker, source)
					}
				}
			}
		})
	}
}

func TestCgoRejectsUnsupportedDeclarations(t *testing.T) {
	for _, prototype := range []string{
		"typedef struct {int a,b;} Pair; Pair f(Pair);",
		"typedef struct {int a,b;} Pair; Pair *f(Pair *);",
		"typedef struct {int a,b;} Pair; void f(Pair (*)(Pair));",
	} {
		h, err := Parse(context.Background(), headerFile(t, prototype), Options{Compiler: compilerTool(t), Functions: []string{"f"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.CgoSource("bindings", "Bindings"); err == nil {
			t.Fatal("unsupported bridge emitted:", prototype)
		}
	}
	h, err := Parse(context.Background(), headerFile(t, "int f(int);"), Options{Compiler: compilerTool(t), Functions: []string{"f"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "_", "type", "C", "abi", "clang", "dylib", "unsafe", "fmt", "int32", "bool", "error", "nil"} {
		if _, err := h.CgoSource("bindings", name); err == nil {
			t.Fatal("invalid binding name accepted:", name)
		}
	}
	h.Functions[0].Signature.Convention = abi.StdCall
	if _, err := h.CgoSource("bindings", "Bindings"); err == nil {
		t.Fatal("unsupported convention emitted")
	}
	h.Functions[0].Signature.Convention = abi.Default
	h.Functions = append(h.Functions, Declaration{Name: "F", Symbol: "F", Signature: h.Functions[0].Signature})
	if _, err := h.CgoSource("bindings", "Bindings"); err == nil {
		t.Fatal("ambiguous method emitted")
	}
}

func TestCgoScalarImports(t *testing.T) {
	h, err := Parse(context.Background(), headerFile(t, "int f(void); void empty(void);"), Options{Compiler: compilerTool(t), Functions: []string{"f", "empty"}})
	if err != nil {
		t.Fatal(err)
	}
	source, err := h.CgoSource("bindings", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "\"unsafe\"") {
		t.Fatal("scalar-only bindings have an unused unsafe import")
	}
}
