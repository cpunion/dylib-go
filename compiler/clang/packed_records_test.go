package clang

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/ramblelabs/dylib-go/abi"
)

func TestPackedRecordBridgesAcrossTargets(t *testing.T) {
	path := headerFile(t, `typedef struct __attribute__((packed)) { signed char tag; double value; short tail; } Packed;
typedef struct { Packed items[2][3]; int count; } Outer;
Packed echo(Packed); Outer nest(Outer); Packed *pointer(Packed *);
typedef Packed (*Entry)(Packed); Entry factory(void); Packed apply(Entry,Packed);`)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"echo", "nest", "pointer", "factory", "apply"}})
			if err != nil {
				t.Fatal(err)
			}
			packed, err := h.LookupRecord("Packed")
			if err != nil || !packed.Packed || !reflect.DeepEqual(packed.Layout, abi.Layout{Size: 11, Alignment: 1, Offsets: []uint64{0, 1, 9}}) {
				t.Fatal("packed compiler layout:", packed, err)
			}
			outer, err := h.LookupRecord("Outer")
			if err != nil || outer.Packed || !reflect.DeepEqual(outer.Layout, abi.Layout{Size: 72, Alignment: 4, Offsets: []uint64{0, 68}}) {
				t.Fatal("nested packed layout:", outer, err)
			}
			source, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"Packed: true", "__attribute__((packed))", "_record0_transport", "_record1_transport", "_from_transport(", "_to_transport(((", "i1 < 3"} {
				if !strings.Contains(string(source), marker) {
					t.Fatalf("missing packed bridge %s", marker)
				}
			}
			if target.arch != "386" {
				if _, err := h.LLGoSource("bindings", "Bindings"); err == nil || !strings.Contains(err.Error(), "requires CgoSource") {
					t.Fatal("direct packed llgo adapter must reject unsupported layout:", err)
				}
			}
			if err := h.Target.CheckHost(); err == nil {
				for _, name := range []string{"echo", "nest", "pointer", "factory", "apply"} {
					if _, err := h.ForHost(name); err == nil || !strings.Contains(err.Error(), "compiled C adapter") {
						t.Fatal("packed dynamic declaration accepted:", name, err)
					}
				}
			}
		})
	}
}

func TestPackedRecordAmbiguousLayouts(t *testing.T) {
	h, err := Parse(context.Background(), headerFile(t, `typedef struct { int value; } Plain;
typedef struct __attribute__((packed)) { int value; } Packed;
Plain plain(Plain); Packed packed(Packed);`), Options{Compiler: compilerTool(t), Functions: []string{"plain", "packed"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.CgoSource("bindings", "Bindings"); err == nil || !strings.Contains(err.Error(), "ambiguous native layouts") {
		t.Fatal("indistinguishable logical records with different native layouts accepted:", err)
	}
}

func TestPackedRecordEquivalentDeclarations(t *testing.T) {
	path := headerFile(t, `typedef struct __attribute__((packed)) { char tag; double value; } First;
typedef struct __attribute__((packed)) { char tag; double value; } Second;
First first(First); Second second(Second);`)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			compiler := compilerTool(t)
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: []string{"first", "second"}})
			if err != nil {
				t.Fatal(err)
			}
			source, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			// Equivalent logical types may share call prototypes, but each emitted
			// record still needs its own transport/conversion declarations.
			text := string(source)
			start := strings.Index(text, "/*") + 2
			end := strings.Index(text[start:], "*/") + start
			if _, err := runCompiler(context.Background(), compiler, []string{"--target=" + target.triple, "-ffreestanding", "-x", "c", "-std=c17", "-fsyntax-only", "-"}, text[start:end], maxDiagnostics); err != nil {
				t.Fatal(err)
			}
		})
	}
}
