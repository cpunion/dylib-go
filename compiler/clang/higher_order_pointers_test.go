package clang

import (
	"context"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/ramblelabs/dylib-go/abi"
)

const higherOrderPointerHeader = `typedef int (*Adder)(int,int);
typedef Adder (*Maker)(void);
typedef Maker (*MetaMaker)(void);
typedef int (*Consumer)(Adder,int,int);
MetaMaker meta_factory(void);
int apply_maker(Maker,int,int);
int apply_consumer(Consumer,Adder,int,int);
Adder *adder_slot(void);
void replace_adder(Adder *,Adder);
`

func TestHigherOrderFunctionPointersAcrossTargets(t *testing.T) {
	path := headerFile(t, higherOrderPointerHeader)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"meta_factory", "apply_maker", "apply_consumer", "adder_slot", "replace_adder"}})
			if err != nil {
				t.Fatal(err)
			}
			factory, _ := h.Lookup("meta_factory")
			meta, err := factory.LookupFunctionPointer(-1)
			if err != nil {
				t.Fatal(err)
			}
			maker, err := meta.LookupFunctionPointer(-1)
			if err != nil {
				t.Fatal(err)
			}
			adder, err := maker.LookupFunctionPointer(-1)
			if err != nil || adder.Signature.Result != abi.I32 || len(adder.Signature.Args) != 2 {
				t.Fatal(adder, err)
			}
			meta.FunctionPointers[0].FunctionPointers[0].Signature.Args[0] = abi.F64
			original, _ := factory.LookupFunctionPointer(-1)
			if original.FunctionPointers[0].FunctionPointers[0].Signature.Args[0] != abi.I32 {
				t.Fatal("nested prototype snapshot was shared")
			}
			slot, _ := h.Lookup("adder_slot")
			pointer, err := slot.LookupFunctionPointer(-1)
			if err != nil || pointer.Indirection != 2 {
				t.Fatal(pointer, err)
			}
			consumer, _ := h.Lookup("apply_consumer")
			outer, _ := consumer.LookupFunctionPointer(0)
			if nested, err := outer.LookupFunctionPointer(0); err != nil || len(nested.Signature.Args) != 2 {
				t.Fatal(nested, err)
			}
			generators := []func(string, string) ([]byte, error){h.GoSource, h.CgoSource}
			if h.Target.Arch != "386" {
				generators = append(generators, h.LLGoSource)
			}
			for _, generate := range generators {
				source, err := generate("bindings", "Bindings")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestHigherOrderPointerValidation(t *testing.T) {
	signature := abi.Signature{Result: abi.I32, Args: []abi.Type{abi.Pointer}}
	for _, layers := range []int{-1, 9} {
		d := Declaration{Signature: signature, FunctionPointers: []FunctionPointer{{Position: 0, Signature: signature, Indirection: layers}}}
		if _, err := d.LookupFunctionPointer(0); err == nil {
			t.Fatal("invalid indirection accepted", layers)
		}
	}
	pointers := []FunctionPointer{{Position: 0, Signature: signature}}
	pointers[0].FunctionPointers = pointers
	d := Declaration{Signature: signature, FunctionPointers: pointers}
	if _, err := d.LookupFunctionPointer(0); err == nil {
		t.Fatal("cyclic function-pointer metadata accepted")
	}
	for _, layers := range []int{3, 8, 9} {
		source := "typedef int (*Fn)(int); int f(Fn" + strings.Repeat("*", layers-1) + ");"
		h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: compilerTool(t), Functions: []string{"f"}})
		if layers == 9 {
			if err == nil {
				t.Fatal("excessive parsed pointer indirection accepted")
			}
			continue
		}
		if err != nil || h.Functions[0].FunctionPointers[0].Indirection != layers {
			t.Fatal(layers, h, err)
		}
	}
}

func TestHigherOrderCallbackPrototypeReuse(t *testing.T) {
	leaf := FunctionPointer{Signature: abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32}}}
	other := leaf.clone()
	other.Position = 1
	other.Signature.Args[0] = abi.F64
	parent := FunctionPointer{Signature: abi.Signature{Result: abi.Void, Args: []abi.Type{abi.Pointer, abi.Pointer}}, FunctionPointers: []FunctionPointer{leaf, other}}
	reordered := parent.clone()
	reordered.FunctionPointers[0], reordered.FunctionPointers[1] = reordered.FunctionPointers[1], reordered.FunctionPointers[0]
	mapper, err := newDirectTypes(nil, "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	if err := mapper.addCallbacks([]Declaration{{FunctionPointers: []FunctionPointer{parent, reordered}}}); err != nil {
		t.Fatal(err)
	}
	if len(mapper.callbacks) != 3 || mapper.callbackIndex(parent) != mapper.callbackIndex(reordered) {
		t.Fatal("equivalent nested prototypes generated separate callback types")
	}
}
