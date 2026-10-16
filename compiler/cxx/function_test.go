package cxx_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/cxx"
	cxxexample "github.com/cpunion/dylib-go/examples/cxx"
)

func TestFunctionPointerManifestValidation(t *testing.T) {
	for name, patch := range map[string]func([]cxx.Class){
		"index":          func(c []cxx.Class) { c[0].Constructors[0].FunctionArgs[0].Index = 1 },
		"negative_index": func(c []cxx.Class) { c[0].Constructors[0].FunctionArgs[0].Index = -1 },
		"scalar_slot":    func(c []cxx.Class) { c[0].Constructors[0].Args[0] = abi.I32 },
		"duplicate": func(c []cxx.Class) {
			c[0].Constructors[0].FunctionArgs = append(c[0].Constructors[0].FunctionArgs, c[0].Constructors[0].FunctionArgs[0])
		},
		"object_collision": func(c []cxx.Class) {
			c[0].Constructors[0].ObjectArgs = []cxx.ObjectArg{{Index: 0, Type: cxx.ObjectType{Name: "example::Counter", Kind: cxx.ObjectPointer}}}
		},
		"variadic":   func(c []cxx.Class) { c[0].Constructors[0].FunctionArgs[0].Signature.Variadic = true },
		"convention": func(c []cxx.Class) { c[0].Constructors[0].FunctionArgs[0].Signature.Convention = abi.StdCall },
		"aggregate":  func(c []cxx.Class) { c[0].Constructors[0].FunctionArgs[0].Signature.Result = abi.Struct },
		"typed_pointee": func(c []cxx.Class) {
			c[0].Constructors[0].FunctionArgs[0].Signature = abi.Signature{Result: abi.Pointer, ResultType: &abi.TypeDesc{Type: abi.Pointer, Elem: &abi.TypeDesc{Type: abi.I32}}}
		},
		"result_transport": func(c []cxx.Class) { c[0].Methods[0].Signature.Result = abi.I32 },
		"result_collision": func(c []cxx.Class) {
			c[0].Methods[0].ObjectResult = &cxx.ObjectType{Name: "example::Counter", Kind: cxx.ObjectPointer}
		},
		"result_signature": func(c []cxx.Class) {
			c[0].Methods[0].FunctionResult = &abi.Signature{Result: abi.I32, Args: []abi.Type{abi.Void}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			classes := cxxexample.FunctionClasses()
			patch(classes)
			if got, err := cxx.Generate("counter.hpp", classes); err == nil || got != nil {
				t.Fatal("invalid native function manifest accepted:", err)
			}
		})
	}
}

func TestNativeCompilerRejectsWrongFunctionPointers(t *testing.T) {
	include, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for name, patch := range map[string]func([]cxx.Class){
		"constructor":     func(c []cxx.Class) { c[0].Constructors[0].FunctionArgs[0].Signature.Result = abi.F64 },
		"method_argument": func(c []cxx.Class) { c[0].Methods[3].FunctionArgs[0].Signature.Args = []abi.Type{abi.I64, abi.I32} },
		"method_result": func(c []cxx.Class) {
			c[0].Methods[0].FunctionResult = &abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			classes := cxxexample.FunctionClasses()
			patch(classes)
			facade, err := cxx.Generate("counter.hpp", classes)
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(t.TempDir(), "invalid.cpp")
			write(t, source, facade.CXXSource)
			output, err := exec.Command(cppTool(t), "-std=c++17", "-fsyntax-only", "-I"+include, source).CombinedOutput()
			if err == nil || !strings.Contains(string(output), "error:") {
				t.Fatalf("incorrect callback signature accepted: %v\n%s", err, output)
			}
		})
	}
}
