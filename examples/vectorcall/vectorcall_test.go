//go:build cgo && windows && (amd64 || 386)

package vectorcall_test

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

	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/clang"
)

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
func TestNativeGeneratedVectorcall(t *testing.T) {
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = os.Getenv("CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	target := "x86_64-w64-windows-gnu"
	if runtime.GOARCH == "386" {
		target = "i686-w64-windows-gnu"
	}
	flags := []string{"--target=" + target}
	if root := os.Getenv("DYLIB_NATIVE_SYSROOT"); root != "" {
		flags = append(flags, "--sysroot="+root, "--rtlib=libgcc")
	}
	names := []string{"vector_unsigned", "vector_bool", "vector_void", "vector_zero", "vector_add", "vector_wide", "vector_mixed", "vector_pair", "vector_hfa", "vector_large", "vector_pointer", "vector_factory", "vector_forward", "vector_apply", "vector_hfa_factory", "vector_hfa_forward"}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: target, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	for _, function := range h.Functions {
		if function.Signature.Convention == abi.VectorCall && !strings.Contains(function.Symbol, "@@") {
			t.Fatal("missing vectorcall linkage decoration", function)
		}
	}
	source, err := h.CgoSource("main", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("bindings.go", source)
	main, err := os.ReadFile("testdata/main.go")
	if err != nil {
		t.Fatal(err)
	}
	write("main.go", main)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	write("go.mod", []byte(fmt.Sprintf("module vectorcall-test\n\ngo 1.23\n\nrequire github.com/cpunion/dylib-go v0.0.0\nreplace github.com/cpunion/dylib-go => %s\n", strconv.Quote(filepath.ToSlash(root)))))
	host := "go"
	if llgoHost {
		host = os.Getenv("DYLIB_LLGO")
		if host == "" {
			host = "llgo"
		}
	}
	binary := filepath.Join(dir, "vectorcall.exe")
	cmd := exec.Command(tool(t, host), "build", "-o", binary, ".")
	cmd.Dir = dir
	// Windows Go 386 normally uses GCC; vectorcall needs a supporting compiler.
	// Go accepts quoted executable paths followed by compiler options in CC.
	cc := strconv.Quote(filepath.ToSlash(compiler))
	for _, flag := range flags {
		cc += " " + strconv.Quote(flag)
	}
	cmd.Env = append(os.Environ(), "GOARCH="+runtime.GOARCH, "CC="+cc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s vectorcall caller: %v\n%s", host, err, out)
	}
	ar := os.Getenv("AR")
	if ar == "" {
		ar = "llvm-ar"
	}
	ar = tool(t, ar)
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			dir := t.TempDir()
			object := filepath.Join(dir, "exports.o")
			args := append([]string{"-std=c17", "-ffreestanding", "-fno-stack-protector", opt, "-c", "testdata/exports.c", "-o", object}, flags...)
			if out, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
				t.Fatalf("vectorcall producer: %v\n%s", err, out)
			}
			archive := filepath.Join(dir, "exports.a")
			if out, err := exec.Command(ar, "rcs", archive, object).CombinedOutput(); err != nil {
				t.Fatalf("vectorcall archive: %v\n%s", err, out)
			}
			library := filepath.Join(dir, "exports.dll")
			args = append([]string{"-shared", object, "-o", library}, flags...)
			if out, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
				t.Fatalf("vectorcall DLL: %v\n%s", err, out)
			}
			for _, input := range []struct{ kind, path string }{{"object", object}, {"archive", archive}, {"library", library}} {
				t.Run(input.kind, func(t *testing.T) {
					if out, err := exec.Command(binary, input.path).CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "42" {
						t.Fatalf("%s vectorcall %s: %v\n%s", host, input.kind, err, out)
					}
				})
			}
		})
	}
}
