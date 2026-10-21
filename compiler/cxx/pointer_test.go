package cxx_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramblelabs/dylib-go/compiler/clang"
	"github.com/ramblelabs/dylib-go/compiler/cxx"
	cxxexample "github.com/ramblelabs/dylib-go/examples/cxx"
)

func TestNativePointerFacadeAcrossTargets(t *testing.T) {
	facade, err := cxx.Generate("pointers.hpp", cxxexample.NativePointerClasses())
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
			if _, err := metadata.CgoSource("bindings", "Bindings"); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(target, "i686-") {
				if _, err := metadata.LLGoSource("bindings", "Bindings"); err != nil {
					t.Fatal(err)
				}
			}
			if output, err := exec.Command(cpp, "--target="+target, "-ffreestanding", "-std=c++17", "-fsyntax-only", "-I"+include, source).CombinedOutput(); err != nil {
				t.Fatalf("C++ pointer producer: %v\n%s", err, output)
			}
		})
	}
}

func TestNativeCompilerRejectsInvalidPointers(t *testing.T) {
	include, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for name, patch := range map[string]func([]cxx.Class){
		"result_const":   func(c []cxx.Class) { c[0].Methods[1].ObjectResult.Const = false },
		"result_level":   func(c []cxx.Class) { c[0].Methods[3].ObjectResult.Name = "example::Number" },
		"argument_type":  func(c []cxx.Class) { c[0].Methods[9].ObjectArgs[0].Type.Name = "example::Character" },
		"argument_const": func(c []cxx.Class) { c[0].Methods[10].ObjectArgs[0].Type.Const = true },
		"slot_const":     func(c []cxx.Class) { c[0].Methods[4].ObjectResult.Const = false },
		"array_extent":   func(c []cxx.Class) { c[0].Methods[5].ObjectResult.Name = "example::OtherRow" },
	} {
		t.Run(name, func(t *testing.T) {
			classes := cxxexample.NativePointerClasses()
			patch(classes)
			facade, err := cxx.Generate("pointers.hpp", classes)
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(t.TempDir(), "invalid.cpp")
			write(t, source, facade.CXXSource)
			output, err := exec.Command(cppTool(t), "-std=c++17", "-fsyntax-only", "-I"+include, source).CombinedOutput()
			if err == nil || !strings.Contains(string(output), "error:") {
				t.Fatalf("invalid native pointer signature accepted: %v\n%s", err, output)
			}
		})
	}
}
