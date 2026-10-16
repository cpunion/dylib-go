package cxx_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/clang"
	"github.com/cpunion/dylib-go/compiler/cxx"
)

func classes() []cxx.Class {
	method := func(name string, result abi.Type, args ...abi.Type) cxx.Method {
		return cxx.Method{Name: name, Symbol: "counter_" + name, Signature: abi.Signature{Result: result, Args: args}}
	}
	add := method("add", abi.I32, abi.I32)
	floating := method("add", abi.F64, abi.F64)
	floating.Symbol, floating.Const = "counter_float", true
	positive, echo, score := method("positive", abi.Bool), method("echo", abi.Pointer, abi.Pointer), method("score", abi.I32, abi.I32)
	positive.Const, echo.Const, score.Const = true, true, true
	live, derived := method("live", abi.I32), method("derived", abi.Pointer, abi.I32)
	live.Static, derived.Static = true, true
	methods := []cxx.Method{add, floating, method("reset", abi.Void, abi.I32), positive, echo, score, live, derived}
	for _, scalar := range []struct {
		name   string
		typeID abi.Type
	}{{"signed8", abi.I8}, {"unsigned8", abi.U8}, {"signed16", abi.I16}, {"unsigned16", abi.U16}, {"unsigned32", abi.U32}, {"signed64", abi.I64}, {"unsigned64", abi.U64}, {"floating32", abi.F32}} {
		m := method(scalar.name, scalar.typeID, scalar.typeID)
		m.Static = true
		methods = append(methods, m)
	}
	return []cxx.Class{
		{Name: "example::Counter", Delete: "counter_delete", Constructors: []cxx.Constructor{{Symbol: "counter_new", Args: []abi.Type{abi.I32}}}, Methods: methods},
		{Name: "example::ThrowingDestructor", Delete: "throwing_delete", Constructors: []cxx.Constructor{{Symbol: "throwing_new"}}},
		{Name: "example::NullAllocation", Delete: "null_delete", Constructors: []cxx.Constructor{{Symbol: "null_new"}}},
		{Name: "example::Derived", Delete: "derived_delete", Constructors: []cxx.Constructor{{Symbol: "derived_new", Args: []abi.Type{abi.I32}}}, Bases: []cxx.Base{{Name: "example::Counter", Symbol: "derived_counter"}}},
		{Name: "example::Diamond", Delete: "diamond_delete", Constructors: []cxx.Constructor{{Symbol: "diamond_new", Args: []abi.Type{abi.I32}}}, Bases: []cxx.Base{{Name: "example::Counter", Symbol: "diamond_counter"}, {Name: "example::VirtualLeft", Symbol: "diamond_left"}}},
		{Name: "example::VirtualLeft", Delete: "left_delete", Bases: []cxx.Base{{Name: "example::Counter", Symbol: "left_counter"}}},
	}
}

func tool(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	return path
}

func clangTool(t *testing.T) string {
	t.Helper()
	for _, variable := range []string{"DYLIB_LLVM_CLANG", "CLANG"} {
		if name := os.Getenv(variable); name != "" {
			return tool(t, name)
		}
	}
	return tool(t, "clang")
}

func cppTool(t *testing.T) string {
	t.Helper()
	if name := os.Getenv("DYLIB_CXX"); name != "" {
		return tool(t, name)
	}
	return tool(t, "clang++")
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestFacadeDeclarationsAcrossTargets(t *testing.T) {
	facade, err := cxx.Generate("counter.hpp", classes())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	header, source := filepath.Join(dir, "facade.h"), filepath.Join(dir, "facade.cpp")
	write(t, header, facade.CHeader)
	write(t, source, facade.CXXSource)
	include, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	compiler, cpp := clangTool(t), cppTool(t)
	for _, target := range []string{"x86_64-unknown-linux-gnu", "aarch64-unknown-linux-gnu", "i686-unknown-linux-gnu", "x86_64-apple-macosx11", "arm64-apple-macosx11", "x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			metadata, err := clang.Parse(context.Background(), header, clang.Options{Compiler: compiler, Target: target, Flags: []string{"-ffreestanding"}, Functions: facade.Functions})
			if err != nil {
				t.Fatal(err)
			}
			create, err := metadata.Lookup("counter_new")
			if err != nil || create.Signature.Result != abi.I32 || !reflect.DeepEqual(create.Signature.Args, []abi.Type{abi.I32, abi.Pointer}) || create.Signature.Convention != abi.CDecl {
				t.Fatal("constructor C boundary:", create, err)
			}
			if _, err := metadata.CgoSource("bindings", "Bindings"); err != nil {
				t.Fatal(err)
			}
			if target != "i686-unknown-linux-gnu" && target != "i686-pc-windows-msvc" {
				if _, err := metadata.LLGoSource("bindings", "Bindings"); err != nil {
					t.Fatal(err)
				}
			}
			if output, err := exec.Command(cpp, "--target="+target, "-ffreestanding", "-std=c++17", "-fsyntax-only", "-I"+include, source).CombinedOutput(); err != nil {
				t.Fatalf("C++ producer: %v\n%s", err, output)
			}
		})
	}
}

func TestFacadeRejectsInvalidManifests(t *testing.T) {
	for _, patch := range []func([]cxx.Class){
		func(c []cxx.Class) { c[0].Name = "fixture::Counter;int injected" },
		func(c []cxx.Class) { c[0].Name = "fixture::class" },
		func(c []cxx.Class) { c[0].Delete = "bad()" },
		func(c []cxx.Class) { c[0].Delete = "and" },
		func(c []cxx.Class) { c[0].Delete = "counter_new" },
		func(c []cxx.Class) { c[3].Bases[0].Name = "example::Counter*" },
		func(c []cxx.Class) { c[3].Bases[0].Symbol = "counter_new" },
		func(c []cxx.Class) { c[3].Bases[0].Symbol = "delete" },
		func(c []cxx.Class) { c[0].Constructors[0].Args[0] = abi.Void },
		func(c []cxx.Class) { c[0].Methods[0].Name = "operator+" },
		func(c []cxx.Class) { c[0].Methods[0].Static, c[0].Methods[0].Const = true, true },
		func(c []cxx.Class) { c[0].Methods[0].Signature.Variadic = true },
		func(c []cxx.Class) { c[0].Methods[0].Signature.Convention = abi.StdCall },
		func(c []cxx.Class) { c[0].Methods[0].Signature.Result = abi.Struct },
		func(c []cxx.Class) { c[0].Methods[0].Signature.ArgTypes = []abi.TypeDesc{{Type: abi.Pointer}} },
		func(c []cxx.Class) {
			c[0].Methods[0].Signature = abi.Signature{Result: abi.Pointer, ResultType: &abi.TypeDesc{Type: abi.Pointer, Elem: &abi.TypeDesc{Type: abi.I32}}}
		},
	} {
		c := classes()
		patch(c)
		if result, err := cxx.Generate("counter.hpp", c); err == nil || result != nil {
			t.Fatal("invalid manifest accepted:", c, err)
		}
	}
	for _, header := range []string{"", "bad\"\n#include \"injected", "C:\\bad.hpp", "bad\x00"} {
		if _, err := cxx.Generate(header, classes()); err == nil {
			t.Fatal("invalid include path accepted:", header)
		}
	}
	if _, err := cxx.Generate("counter.hpp", nil); err == nil {
		t.Fatal("empty manifest accepted")
	}
	first, _ := cxx.Generate("a header with spaces.hpp", classes())
	second, _ := cxx.Generate("a header with spaces.hpp", classes())
	if !bytes.Equal(first.CHeader, second.CHeader) || !bytes.Equal(first.CXXSource, second.CXXSource) {
		t.Fatal("generation is not deterministic")
	}
}

func TestNativeCompilerRejectsInvalidBaseViews(t *testing.T) {
	include, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, receiver, base string }{
		{"private", "example::PrivateCounter", "example::Counter"},
		{"ambiguous", "example::AmbiguousCounter", "example::Counter"},
		{"unrelated", "example::Counter", "example::ThrowingDestructor"},
		{"downcast", "example::Counter", "example::Derived"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facade, err := cxx.Generate("counter.hpp", []cxx.Class{{Name: tc.receiver, Delete: "remove_object", Bases: []cxx.Base{{Name: tc.base, Symbol: "base_view"}}}})
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(t.TempDir(), "invalid.cpp")
			write(t, source, facade.CXXSource)
			output, err := exec.Command(cppTool(t), "-std=c++17", "-fsyntax-only", "-I"+include, source).CombinedOutput()
			if err == nil || !strings.Contains(string(output), "*view") {
				t.Fatalf("invalid base view accepted or wrong diagnostic: %v\n%s", err, output)
			}
		})
	}
}

func TestNativeCompilerRejectsWrongMemberSignature(t *testing.T) {
	c := classes()
	c[0].Methods[0].Signature.Result = abi.F32
	facade, err := cxx.Generate("counter.hpp", c)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "wrong.cpp")
	write(t, source, facade.CXXSource)
	include, _ := filepath.Abs("testdata")
	output, err := exec.Command(cppTool(t), "-std=c++17", "-fsyntax-only", "-I"+include, source).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "static_cast") {
		t.Fatalf("wrong member signature was not rejected: %v\n%s", err, output)
	}
}
