//go:build cgo && (linux || darwin || windows)

package tailpadding

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
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

func TestUnnamedTailPaddingCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_TYPEDEF_CLANG")
	if compiler == "" {
		compiler = os.Getenv("DYLIB_LLVM_CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	names := []string{"echo_float", "echo_double", "echo_integer", "echo_array", "echo_triple", "echo_parent", "float_overflow", "double_overflow", "integer_overflow", "float_variadic", "double_variadic", "float_factory", "forward_float", "double_pointer", "mutate_double", "read_double"}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{9, 10} {
		h.Functions[index], err = h.Functions[index].WithTail(abi.I8, abi.F32)
		if err != nil {
			t.Fatal(err)
		}
	}

	fresh, err := h.CgoSource("tailpadding", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated unnamed tail padding bindings are stale")
	}
	var archiver string
	candidates := []string{filepath.Join(filepath.Dir(compiler), "llvm-ar"), "llvm-ar", "ar"}
	if override := os.Getenv("DYLIB_LLVM_AR"); override != "" {
		candidates = []string{override}
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			archiver = path
			break
		}
	}
	if archiver == "" {
		if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
			t.Fatal("native archiver unavailable")
		}
		t.Skip("native archiver unavailable")
	}
	compile := func(t *testing.T, tool string, args ...string) {
		t.Helper()
		if output, err := exec.Command(tool, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", tool, err, output)
		}
	}
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			dir := t.TempDir()
			object, archive := filepath.Join(dir, "global.o"), filepath.Join(dir, "global.a")
			flags := []string{"--target=" + h.Target.Triple, optimization, "-fno-stack-protector"}
			if runtime.GOOS != "windows" {
				flags = append(flags, "-fPIC")
			}
			compile(t, compiler, append(flags, "-c", "testdata/exports.c", "-o", object)...)
			compile(t, archiver, "rcs", archive, object)
			library, linkFlags := filepath.Join(dir, "global.so"), []string{"-shared", "-fPIC"}
			switch runtime.GOOS {
			case "darwin":
				library, linkFlags = filepath.Join(dir, "global.dylib"), []string{"-dynamiclib"}
			case "windows":
				library, linkFlags = filepath.Join(dir, "global.dll"), []string{"-shared", "-Wl,--export-all-symbols"}
			}
			if runtime.GOARCH == "386" {
				if runtime.GOOS == "linux" {
					linkFlags = append(linkFlags, "-m32")
				} else if runtime.GOOS == "windows" {
					linkFlags = append(linkFlags, "--target=i686-w64-windows-gnu")
					if root := os.Getenv("DYLIB_NATIVE_SYSROOT"); root != "" {
						linkFlags = append(linkFlags, "--sysroot="+root, "--rtlib=libgcc")
					}
				}
			}
			// Retain the producer ABI selected during parsing, including MSVC
			// i386's indirect aligned arguments when a MinGW driver links the DLL.
			compile(t, compiler, append(linkFlags, object, "-o", library)...)
			for _, input := range []struct{ name, path string }{{"object", object}, {"archive", archive}, {"library", library}} {
				t.Run(input.name, func(t *testing.T) { checkCalls(t, input.path) })
			}
		})
	}
}

func checkCalls(t *testing.T, path string) {
	t.Helper()
	if got, err := Call(path); err != nil || got != 42 {
		t.Fatal(got, err)
	}
	session, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	b, err := NewBindings(session)
	if err != nil {
		t.Fatal(err)
	}
	check := func(got, want any, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal(got, want, err)
		}
	}
	f := BindingsRecord0{Float_value: -123.75}
	v0, err := b.Echo_float(f)
	check(v0, f, err)
	d := BindingsRecord1{Double_value: 98765.125}
	v1, err := b.Echo_double(d)
	check(v1, d, err)
	i := BindingsRecord2{Integer_value: -0x123456789abcdef}
	v2, err := b.Echo_integer(i)
	check(v2, i, err)
	a := BindingsRecord3{Array_values: [1]float32{1.25}}
	v3, err := b.Echo_array(a)
	check(v3, a, err)
	triple := BindingsRecord4{First: 1.25, Second: -123.75, Third: 234.5}
	v4, err := b.Echo_triple(triple)
	check(v4, triple, err)
	parent := BindingsRecord5{Parent_tag: -3, Parent_values: [2]BindingsRecord0{f, {Float_value: 234.5}}}
	v5, err := b.Echo_parent(parent)
	check(v5, parent, err)
	number, err := b.Float_overflow(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord0{Float_value: 5}, 1)
	check(number, float64(42), err)
	number, err = b.Double_overflow(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord1{Double_value: 5}, 1)
	check(number, float64(42), err)
	integer, err := b.Integer_overflow(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord2{Integer_value: 5}, 1)
	check(integer, int64(42), err)
	number, err = b.Float_variadic(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord0{Float_value: 2}, 1, 3)
	check(number, float64(42), err)
	number, err = b.Double_variadic(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord1{Double_value: 2}, 1, 3)
	check(number, float64(42), err)
	entry, err := b.Float_factory()
	if err != nil || entry == nil {
		t.Fatal(entry, err)
	}
	v0, err = b.Forward_float(entry, f)
	check(v0, f, err)
	pointer, err := b.Double_pointer(d)
	if err != nil || pointer == nil {
		t.Fatal(pointer, err)
	}
	pointer2, err := b.Mutate_double(pointer)
	check(pointer2, pointer, err)
	v1, err = b.Read_double(pointer)
	d.Double_value += 2
	check(v1, d, err)
	pointer2, err = b.Mutate_double(nil)
	if err != nil || pointer2 != nil {
		t.Fatal(pointer2, err)
	}
}
