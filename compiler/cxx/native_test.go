//go:build cgo && (linux || darwin || windows)

package cxx_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/compiler/clang"
	"github.com/cpunion/dylib-go/compiler/cxx"
	cxxexample "github.com/cpunion/dylib-go/examples/cxx"
)

func prepare(t *testing.T, facade *cxx.Facade) (string, *clang.Header) {
	t.Helper()
	dir := t.TempDir()
	source, header := filepath.Join(dir, "facade.cpp"), filepath.Join(dir, "facade.h")
	write(t, source, facade.CXXSource)
	write(t, header, facade.CHeader)
	metadata, err := clang.Parse(context.Background(), header, clang.Options{Compiler: clangTool(t), Functions: facade.Functions})
	if err != nil {
		t.Fatal(err)
	}
	return source, metadata
}

func shared(t *testing.T, source, optimization string) string {
	t.Helper()
	args := []string{"-std=c++17", optimization, "-shared"}
	library := filepath.Join(t.TempDir(), "counter.so")
	switch runtime.GOOS {
	case "darwin":
		library = filepath.Join(t.TempDir(), "counter.dylib")
		args = []string{"-std=c++17", optimization, "-dynamiclib"}
	case "windows":
		library = filepath.Join(t.TempDir(), "counter.dll")
	}
	if runtime.GOOS != "windows" {
		args = append(args, "-fPIC")
	}
	if runtime.GOARCH == "386" {
		if runtime.GOOS == "linux" {
			args = append(args, "-m32")
		} else if runtime.GOOS == "windows" {
			args = append(args, "--target=i686-w64-windows-gnu", "-stdlib=libstdc++")
			if root := os.Getenv("DYLIB_NATIVE_SYSROOT"); root != "" {
				args = append(args, "--sysroot="+root, "--rtlib=libgcc")
			}
		}
	}
	args = append(args, source, "-o", library)
	if output, err := exec.Command(cppTool(t), args...).CombinedOutput(); err != nil {
		t.Fatalf("C++ shared producer: %v\n%s", err, output)
	}
	return library
}

func TestNativeGeneratedCgoClassFacade(t *testing.T) {
	testGeneratedClassFacade(t, false)
}

func TestNativeGeneratedDirectLLGoClassFacade(t *testing.T) {
	if !llgoHost {
		t.Skip("the direct caller requires an llgo host")
	}
	testGeneratedClassFacade(t, true)
}

func testGeneratedClassFacade(t *testing.T, direct bool) {
	testGeneratedFacade(t, direct, "testdata/cgo_main.go", cxxexample.Generate)
}

func TestNativeGeneratedCgoObjectFacade(t *testing.T) {
	testGeneratedFacade(t, false, "testdata/objects_main.go", generateObjects)
}

func TestNativeGeneratedDirectLLGoObjectFacade(t *testing.T) {
	if !llgoHost {
		t.Skip("the direct caller requires an llgo host")
	}
	testGeneratedFacade(t, true, "testdata/objects_main.go", generateObjects)
}

func generateObjects(header string) (*cxx.Facade, error) {
	return cxx.Generate(header, cxxexample.ObjectClasses())
}

func TestNativeGeneratedCgoFunctionFacade(t *testing.T) {
	testGeneratedFacade(t, false, "testdata/functions_main.go", generateFunctions)
}

func TestNativeGeneratedDirectLLGoFunctionFacade(t *testing.T) {
	if !llgoHost {
		t.Skip("the direct caller requires an llgo host")
	}
	testGeneratedFacade(t, true, "testdata/functions_main.go", generateFunctions)
}

func generateFunctions(header string) (*cxx.Facade, error) {
	classes := append(cxxexample.FunctionClasses(), cxxexample.ObjectClasses()...)
	return cxx.Generate(header, classes)
}

func testGeneratedFacade(t *testing.T, direct bool, fixture string, render func(string) (*cxx.Facade, error)) {
	t.Helper()
	headerPath, err := filepath.Abs("testdata/counter.hpp")
	if err != nil {
		t.Fatal(err)
	}
	facade, err := render(filepath.ToSlash(headerPath))
	if err != nil {
		t.Fatal(err)
	}
	source, metadata := prepare(t, facade)
	generate := metadata.CgoSource
	if direct {
		generate = metadata.LLGoSource
	}
	bindings, err := generate("main", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "bindings.go"), bindings)
	main, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "main.go"), main)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "go.mod"), []byte(fmt.Sprintf("module cxx-facade-test\n\ngo 1.23\n\nrequire github.com/cpunion/dylib-go v0.0.0\nreplace github.com/cpunion/dylib-go => %s\n", strconv.Quote(filepath.ToSlash(root)))))
	compiler := "go"
	if llgoHost {
		compiler = os.Getenv("DYLIB_LLGO")
		if compiler == "" {
			compiler = "llgo"
		}
	}
	binary := filepath.Join(dir, "counter-runner")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	// This compiles freshly generated caller code, without the libffi tag.
	cmd := exec.Command(tool(t, compiler), "build", "-o", binary, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOARCH="+runtime.GOARCH)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated %s caller: %v\n%s", compiler, err, output)
	}
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			library := shared(t, source, optimization)
			if output, err := exec.Command(binary, library).CombinedOutput(); err != nil || strings.TrimSpace(string(output)) != "42" {
				t.Fatalf("generated %s C++ call: %v\n%s", compiler, err, output)
			}
		})
	}
}
