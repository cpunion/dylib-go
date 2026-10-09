package clang

import (
	"context"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestDirectRecordGeneration(t *testing.T) {
	path := headerFile(t, recordHeader)
	for _, target := range targets {
		if target.arch == "386" {
			continue
		}
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"echo", "nest", "mutate"}})
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
			for _, marker := range []string{"BindingsRecord0", "[2]BindingsRecord0", "[2][3]int32", "Tag int8", "unsafe.Sizeof", "unsafe.Alignof", "unsafe.Offsetof", "Target.CheckHost", "*BindingsRecord0"} {
				if !strings.Contains(strings.Join(strings.Fields(string(source)), " "), marker) {
					t.Fatalf("missing %s:\n%s", marker, source)
				}
			}
		})
	}
	h, err := Parse(context.Background(), headerFile(t, "struct S{int a,A;}; struct S f(struct S);"), Options{Compiler: compilerTool(t), Functions: []string{"f"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
		t.Fatal("ambiguous record fields emitted")
	}
	h.Records = nil
	if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
		t.Fatal("missing compiler record emitted")
	}
}
