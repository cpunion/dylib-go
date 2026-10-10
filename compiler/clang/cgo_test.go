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
		"typedef struct {int a,b;} Pair; Pair *f(Pair *);",
		"typedef struct {int a,b;} Pair; void f(Pair *(*)(Pair *));",
	} {
		h, err := Parse(context.Background(), headerFile(t, prototype), Options{Compiler: compilerTool(t), Functions: []string{"f"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.CgoSource("bindings", "Bindings"); err == nil {
			t.Fatal("unsupported bridge emitted:", prototype)
		}
	}
	h, err := Parse(context.Background(), headerFile(t, "int f(int);"), Options{Compiler: compilerTool(t), Target: "x86_64-unknown-linux-gnu", Functions: []string{"f"}})
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

func TestCgoRecordsAcrossTargets(t *testing.T) {
	path := headerFile(t, "typedef struct {short a; double b;} Pair; typedef struct {Pair items[2]; int matrix[2][3]; _Bool truth; void *pointer;} Outer; Outer f(Outer); Pair apply(Pair (*)(Pair),Pair);")
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"f", "apply"}})
			if err != nil {
				t.Fatal(err)
			}
			// C typedef dependencies must work independently of metadata order.
			h.Records[0], h.Records[1] = h.Records[1], h.Records[0]
			source, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"record1 f0[2]", "int32_t f1[2][3]", "_Bool f2", "void * f3", "_Alignof", "offsetof", "Record0ToC", "Record0FromC", "for i0 := range 2", "for i1 := range 3", "compiler/cgo layout mismatch"} {
				if !strings.Contains(string(source), marker) {
					t.Fatalf("missing %s:\n%s", marker, source)
				}
			}
			if strings.Contains(string(source), "unsafe.Sizeof") || strings.Contains(string(source), "unsafe.Offsetof") {
				t.Fatal("cgo record bindings must not depend on Go struct layout")
			}
			inner := strings.Index(string(source), "} dylib_go_42696e64696e6773_record1;")
			outer := strings.Index(string(source), "} dylib_go_42696e64696e6773_record0;")
			if inner < 0 || inner >= outer {
				t.Fatal("C record dependencies emitted out of order")
			}
		})
	}
}

func TestCgoCallingConventionTargets(t *testing.T) {
	path := headerFile(t, "int f(int (*)(int),int);")
	for _, target := range targets {
		for _, convention := range []abi.Convention{abi.StdCall, abi.FastCall} {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"f"}})
			if err != nil {
				t.Fatal(err)
			}
			for _, outer := range []bool{false, true} {
				h.Functions[0].FunctionPointers[0].Signature.Convention = convention
				if outer {
					h.Functions[0].Signature.Convention = convention
				}
				source, err := h.CgoSource("bindings", "Bindings")
				qualified := target.os == "windows" && target.arch == "386"
				if (err == nil) != qualified {
					t.Fatalf("%s convention %d outer %t: %v", target.triple, convention, outer, err)
				}
				if qualified {
					attribute := "stdcall"
					if convention == abi.FastCall {
						attribute = "fastcall"
					}
					if !strings.Contains(string(source), "__attribute__(("+attribute+"))") {
						t.Fatal("native convention lost")
					}
				}
			}
		}
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
