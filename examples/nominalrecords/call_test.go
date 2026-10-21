//go:build cgo && (linux || darwin || windows)

package nominalrecords

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/ramblelabs/dylib-go/abi"
	"github.com/ramblelabs/dylib-go/compiler/clang"
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

func TestNominalRecordCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_TYPEDEF_CLANG")
	if compiler == "" {
		compiler = os.Getenv("DYLIB_LLVM_CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	names, err := os.ReadFile("testdata/functions.txt")
	if err != nil {
		t.Fatal(err)
	}
	functions := strings.Fields(string(names))
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		functions = append(functions, "echo_wide", "echo_small")
	}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: functions, NominalRecords: true})
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range h.Functions {
		if d.Name == "packed_variable" {
			h.Functions[i], err = d.WithTail(abi.I8, abi.F32)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := h.CgoSource("nominalrecords", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated nominal record bindings are stale")
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
		t.Fatal("example", got, err)
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
	plain := BindingsRecord0{X: 40.5, Y: 20}
	plainWant := BindingsRecord0{X: 42, Y: 21}
	packed := BindingsRecord1{X: 40.5, Y: 20}
	packedWant := BindingsRecord1{X: 42, Y: 21}
	aligned := BindingsRecord2{X: 40.5, Y: 20}
	alignedWant := BindingsRecord2{X: 42, Y: 21}
	check := func(name string, got, want any, err error) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("got %+v, %v; want %+v", got, err, want)
			}
		})
	}
	v0, err := b.Echo_plain(plain)
	check("plain", v0, plainWant, err)
	v1, err := b.Echo_packed(packed)
	check("packed", v1, packedWant, err)
	v2, err := b.Echo_aligned(aligned)
	check("aligned", v2, alignedWant, err)
	v0, err = b.Echo_alias(plain)
	check("alias", v0, plainWant, err)
	envelope := BindingsRecord3{A: [2]BindingsRecord0{plain, plain}, B: [2]BindingsRecord1{packed, packed}, C: [2]BindingsRecord2{aligned, aligned}}
	want := BindingsRecord3{A: [2]BindingsRecord0{plainWant, plainWant}, B: [2]BindingsRecord1{packedWant, packedWant}, C: [2]BindingsRecord2{alignedWant, alignedWant}}
	v3, err := b.Echo_envelope(envelope)
	check("nested_arrays", v3, want, err)
	v1, err = b.Packed_variable(packed, -8, 1.5)
	check("variadic_prefix", v1, BindingsRecord1{X: 42, Y: 12}, err)
	checkUnnamed(t, b)
	owner, err := session.Resolve("packed_factory")
	if err != nil {
		t.Fatal(err)
	}
	// Pin the image while its borrowed native code and storage addresses are used.
	err = owner.WithAddress(func(uintptr) error {
		entry, err := b.Packed_factory()
		if err != nil {
			return err
		}
		if entry == nil {
			return fmt.Errorf("nil factory")
		}
		v1, err = b.Apply_packed(entry, packed)
		check("factory", v1, packedWant, err)
		v1, err = b.Apply_packed(nil, packed)
		check("null_factory", v1, packed, err)
		p0, err := b.Plain_pointer(plain)
		if err != nil {
			return err
		}
		if p0 == nil {
			return fmt.Errorf("nil plain pointer")
		}
		v0, err = b.Read_plain(p0)
		check("plain_pointer", v0, plainWant, err)
		p1, err := b.Packed_pointer(packed)
		if err != nil {
			return err
		}
		if p1 == nil {
			return fmt.Errorf("nil packed pointer")
		}
		v1, err = b.Read_packed(p1)
		check("packed_pointer", v1, packedWant, err)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
