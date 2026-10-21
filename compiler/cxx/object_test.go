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

func TestInvalidObjectManifests(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch func([]cxx.Class)
	}{
		{"negative_index", func(c []cxx.Class) { c[0].Methods[0].ObjectArgs[0].Index = -1 }},
		{"large_index", func(c []cxx.Class) { c[0].Methods[0].ObjectArgs[0].Index = 1 }},
		{"duplicate_index", func(c []cxx.Class) { m := &c[0].Methods[0]; m.ObjectArgs = append(m.ObjectArgs, m.ObjectArgs[0]) }},
		{"scalar_transport", func(c []cxx.Class) { c[0].Methods[0].Signature.Args[0] = abi.I32 }},
		{"invalid_name", func(c []cxx.Class) { c[0].Methods[0].ObjectArgs[0].Type.Name = "example::Counter*" }},
		{"unspecified_kind", func(c []cxx.Class) { c[0].Methods[0].ObjectArgs[0].Type.Kind = 0 }},
		{"unknown_kind", func(c []cxx.Class) { c[0].Methods[0].ObjectArgs[0].Type.Kind = 99 }},
		{"const_value", func(c []cxx.Class) { c[0].Methods[1].ObjectArgs[0].Type.Const = true }},
		{"constructor_index", func(c []cxx.Class) { c[0].Constructors[1].ObjectArgs[0].Index = 1 }},
		{"result_transport", func(c []cxx.Class) { c[0].Methods[3].Signature.Result = abi.I32 }},
		{"result_name", func(c []cxx.Class) { c[0].Methods[3].ObjectResult.Name = "bad;injected" }},
		{"result_kind", func(c []cxx.Class) { c[0].Methods[3].ObjectResult.Kind = 0 }},
		{"result_const_value", func(c []cxx.Class) { c[0].Methods[3].ObjectResult.Const = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			classes := cxxexample.ObjectClasses()
			tc.patch(classes)
			if result, err := cxx.Generate("counter.hpp", classes); err == nil || result != nil {
				t.Fatal("invalid native object contract accepted:", err)
			}
		})
	}
	if _, err := cxx.Generate("counter.hpp", []cxx.Class{{Name: "example::Counter"}}); err == nil {
		t.Fatal("manifest without any exports accepted")
	}
}

func TestNativeCompilerRejectsInvalidObjectContracts(t *testing.T) {
	include, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, diagnostic string
		classes          []cxx.Class
	}{
		{"throwing_value_destructor", "class values require noexcept destruction", []cxx.Class{{Name: "example::UnstableValue", Methods: []cxx.Method{{Name: "take", Symbol: "take", Static: true, Signature: abi.Signature{Result: abi.I32, Args: []abi.Type{abi.Pointer}}, ObjectArgs: []cxx.ObjectArg{{Index: 0, Type: cxx.ObjectType{Name: "example::UnstableValue", Kind: cxx.ObjectValue}}}}}}}},
		{"throwing_result_destructor", "class values require noexcept destruction", []cxx.Class{{Name: "example::UnstableValue", Methods: []cxx.Method{{Name: "make", Symbol: "make", Static: true, Signature: abi.Signature{Result: abi.Pointer}, ObjectResult: &cxx.ObjectType{Name: "example::UnstableValue", Kind: cxx.ObjectValue}}}}}},
		{"noncopyable_argument", "deleted", []cxx.Class{{Name: "example::Token", Methods: []cxx.Method{{Name: "take", Symbol: "take", Static: true, Signature: abi.Signature{Result: abi.I32, Args: []abi.Type{abi.Pointer}}, ObjectArgs: []cxx.ObjectArg{{Index: 0, Type: cxx.ObjectType{Name: "example::Token", Kind: cxx.ObjectValue}}}}}}}},
		{"wrong_reference_const", "static_cast", func() []cxx.Class {
			c := cxxexample.ObjectClasses()
			c[0].Methods[0].ObjectArgs[0].Type.Const = false
			return c
		}()},
		{"wrong_native_result", "static_cast", func() []cxx.Class {
			c := cxxexample.ObjectClasses()
			c[0].Methods[3].ObjectResult.Name = "example::Token"
			return c
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facade, err := cxx.Generate("counter.hpp", tc.classes)
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(t.TempDir(), "invalid.cpp")
			write(t, source, facade.CXXSource)
			output, err := exec.Command(cppTool(t), "-std=c++17", "-fsyntax-only", "-I"+include, source).CombinedOutput()
			if err == nil || !strings.Contains(string(output), tc.diagnostic) {
				t.Fatalf("invalid native contract accepted or wrong diagnostic: %v\n%s", err, output)
			}
		})
	}
}
