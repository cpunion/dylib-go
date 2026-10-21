package cxx_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramblelabs/dylib-go/abi"
	"github.com/ramblelabs/dylib-go/compiler/cxx"
	cxxexample "github.com/ramblelabs/dylib-go/examples/cxx"
)

func TestCallableManifestValidation(t *testing.T) {
	for _, name := range []string{"operator ()", "operator+", "operator[]", "operator() const", "operator(); injected()", "operator()\n"} {
		t.Run(name, func(t *testing.T) {
			classes := cxxexample.CallableClasses()
			classes[0].Methods[0].Name = name
			if facade, err := cxx.Generate("callables.hpp", classes); err == nil || facade != nil {
				t.Fatal("invalid operator name accepted", err)
			}
		})
	}
	t.Run("static", func(t *testing.T) {
		classes := cxxexample.CallableClasses()
		classes[0].Methods[0].Static, classes[0].Methods[0].Const = true, false
		if facade, err := cxx.Generate("callables.hpp", classes); err == nil || facade != nil {
			t.Fatal("C++17 static call operator accepted", err)
		}
	})
}

func TestNativeCompilerRejectsInvalidCallables(t *testing.T) {
	include, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for name, patch := range map[string]func([]cxx.Class){
		"const_mutable_lambda": func(c []cxx.Class) { c[1].Methods[0].Const = true },
		"wrong_arguments":      func(c []cxx.Class) { c[0].Methods[0].Signature.Args[0] = abi.F64 },
		"wrong_result":         func(c []cxx.Class) { c[0].Methods[0].Signature.Result = abi.I64 },
		"copy_move_only": func(c []cxx.Class) {
			c[2].Constructors = []cxx.Constructor{{Symbol: "invalid_copy", Args: []abi.Type{abi.Pointer},
				ObjectArgs: []cxx.ObjectArg{{Index: 0, Type: cxx.ObjectType{Name: c[2].Name, Kind: cxx.ObjectReference, Const: true}}}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			classes := cxxexample.CallableClasses()
			patch(classes)
			facade, err := cxx.Generate("callables.hpp", classes)
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(t.TempDir(), "invalid.cpp")
			write(t, source, facade.CXXSource)
			output, err := exec.Command(cppTool(t), "-std=c++17", "-fsyntax-only", "-I"+include, source).CombinedOutput()
			if err == nil || !strings.Contains(string(output), "error:") {
				t.Fatalf("invalid callable signature accepted: %v\n%s", err, output)
			}
		})
	}
}
