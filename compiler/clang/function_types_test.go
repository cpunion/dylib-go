package clang

import (
	"context"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

const functionPointerHeader = `typedef struct {int a,b;} Pair;
typedef int (*Adder)(int,int);
typedef Pair (*PairFn)(Pair);
int apply(Adder,int,int);
int direct(int (*)(int,int),int,int);
int decayed(int callback(int,int),int,int);
Adder factory(void);
int (*anonymous_factory(void))(int,int);
int pair_apply(PairFn,Pair);
void pointer_apply(void (*)(Pair *),Pair *);
typedef int (*Variable)(int,...);
Variable variable_factory(void);
`

func TestFunctionPointersAcrossTargets(t *testing.T) {
	path := headerFile(t, functionPointerHeader)
	names := []string{"apply", "direct", "decayed", "factory", "anonymous_factory", "pair_apply", "pointer_apply", "variable_factory"}
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: names})
			if err != nil {
				t.Fatal(err)
			}
			want := abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}, Convention: abi.CDecl}
			for _, name := range names[:5] {
				d, err := h.Lookup(name)
				if err != nil {
					t.Fatal(err)
				}
				position := 0
				if strings.Contains(name, "factory") {
					position = -1
				}
				p, err := d.LookupFunctionPointer(position)
				if err != nil || !reflect.DeepEqual(p.Signature, want) {
					t.Fatalf("%s: %+v, %v", name, p, err)
				}
				p.Signature.Args[0] = abi.F64
				original, _ := d.LookupFunctionPointer(position)
				if original.Signature.Args[0] != abi.I32 {
					t.Fatal("function pointer snapshot was shared")
				}
			}
			pair, _ := h.Lookup("pair_apply")
			p, err := pair.LookupFunctionPointer(0)
			if err != nil || p.Signature.Result != abi.Struct || p.Signature.Args[0] != abi.Struct {
				t.Fatal(p, err)
			}
			p.Signature.ResultType.Fields[0].Name = "changed"
			original, _ := h.Lookup("pair_apply")
			if original.FunctionPointers[0].Signature.ResultType.Fields[0].Name != "a" {
				t.Fatal("nested snapshot was shared")
			}
			pointer, _ := h.Lookup("pointer_apply")
			if pointer.FunctionPointers[0].Signature.ArgTypes[0].Elem.Type != abi.Struct {
				t.Fatal("typed callback pointer lost")
			}
			variable, _ := h.Lookup("variable_factory")
			v, err := variable.LookupFunctionPointer(-1)
			if err != nil {
				t.Fatal(err)
			}
			v, err = v.WithTail(abi.I8, abi.F32)
			if err != nil || !v.Signature.Variadic || v.Signature.FixedArgs != 1 || len(v.Signature.Args) != 3 {
				t.Fatal(v, err)
			}
			source, err := h.GoSource("bindings", "Declarations")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFunctionPointerConventionsAndValidation(t *testing.T) {
	compiler := compilerTool(t)
	path := headerFile(t, "typedef int (__attribute__((stdcall)) *Std)(int,int); typedef int (__attribute__((fastcall)) *Fast)(int,int); int f(Std,Fast);")
	h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: "i686-pc-windows-msvc", Functions: []string{"f"}})
	if err != nil {
		t.Fatal(err)
	}
	d := h.Functions[0]
	if d.Signature.Convention != abi.CDecl || d.FunctionPointers[0].Signature.Convention != abi.StdCall || d.FunctionPointers[1].Signature.Convention != abi.FastCall {
		t.Fatal(d)
	}
	for _, source := range []string{
		"int f(int (*callback)());",
		"int f(int (**callback)(int));",
		"int f(int (*callback)(int (*)(int)));",
		"int f(int (__attribute__((vectorcall)) *callback)(int));",
		"typedef int I __attribute__((aligned(16))); int f(I (*callback)(I));",
	} {
		if _, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: compiler, Target: "i686-pc-windows-msvc", Functions: []string{"f"}}); err == nil {
			t.Fatal("unsupported function pointer accepted:", source)
		}
	}
	if _, err := d.LookupFunctionPointer(-1); err == nil {
		t.Fatal("missing pointer found")
	}
	for _, position := range []int{-2, 2} {
		bad := d.clone()
		bad.FunctionPointers[0].Position = position
		if _, err := bad.LookupFunctionPointer(position); err == nil {
			t.Fatal("invalid position accepted")
		}
	}
	bad := d.clone()
	bad.FunctionPointers[1].Position = 0
	if _, err := bad.LookupFunctionPointer(0); err == nil {
		t.Fatal("duplicate position accepted")
	}
	bad = d.clone()
	bad.Signature.Args[0] = abi.I32
	if _, err := bad.LookupFunctionPointer(0); err == nil {
		t.Fatal("non-pointer position accepted")
	}
	bad = d.clone()
	cycle := abi.TypeDesc{Type: abi.Pointer}
	cycle.Elem = &cycle
	bad.FunctionPointers[0].Signature = abi.Signature{Result: abi.Pointer, ResultType: &cycle}
	if _, err := bad.LookupFunctionPointer(0); err == nil {
		t.Fatal("cyclic pointer metadata accepted")
	}
}
