//go:build cgo && (linux || darwin || windows)

package packedrecords

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	dylib "github.com/cpunion/dylib-go"
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

func TestPackedCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	names := []string{"echo_packed", "echo_envelope", "packed_factory", "apply_packed", "packed_pointer", "mutate_packed", "sum_packed", "packed_variable"}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	h.Functions[7], err = h.Functions[7].WithTail(abi.I8, abi.F32)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := h.CgoSource("packedrecords", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated packed bindings are stale")
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
			object, archive := filepath.Join(dir, "packed.o"), filepath.Join(dir, "packed.a")
			flags := []string{"--target=" + h.Target.Triple, optimization, "-fno-stack-protector"}
			if runtime.GOOS != "windows" {
				flags = append(flags, "-fPIC")
			}
			compile(t, compiler, append(flags, "-c", "testdata/exports.c", "-o", object)...)
			compile(t, archiver, "rcs", archive, object)
			library, linkFlags := filepath.Join(dir, "packed.so"), []string{"-shared", "-fPIC"}
			switch runtime.GOOS {
			case "darwin":
				library, linkFlags = filepath.Join(dir, "packed.dylib"), []string{"-dynamiclib"}
			case "windows":
				library, linkFlags = filepath.Join(dir, "packed.dll"), []string{"-shared", "-Wl,--export-all-symbols"}
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
			compile(t, compiler, append(linkFlags, optimization, "testdata/exports.c", "-o", library)...)
			for _, input := range []struct{ name, path string }{{"object", object}, {"archive", archive}, {"library", library}} {
				t.Run(input.name, func(t *testing.T) { checkCalls(t, input.path) })
			}
		})
	}
}

func checkCalls(t *testing.T, path string) {
	t.Helper()
	if got, err := Call(path); err != nil || got != 42 {
		t.Fatal("packed example:", got, err)
	}
	s, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	b, err := NewBindings(s)
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, got, want any, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: %+v, %v; want %+v", name, got, err, want)
		}
	}
	owner, err := s.Resolve("echo_packed")
	if err != nil {
		t.Fatal(err)
	}
	err = owner.WithAddress(func(uintptr) error {
		native, err := b.Packed_pointer()
		if err != nil || native == nil {
			t.Fatal("native packed storage:", native, err)
		}
		p := BindingsRecord0{Tag: -128, Value: 20.5, Tail: -32767, Truth: true, Context: native}
		want := BindingsRecord0{Tag: -127, Value: 22, Tail: -32768, Truth: false, Context: native}
		got, err := b.Echo_packed(p)
		check("unaligned/narrow/Boolean/pointer fields", got, want, err)
		fn, err := b.Packed_factory()
		if err != nil || fn == nil {
			t.Fatal("packed native factory:", fn, err)
		}
		got, err = b.Apply_packed(fn, p)
		check("packed function-pointer call", got, want, err)
		envelope := BindingsRecord1{Items: [2][2]BindingsRecord0{{p, p}, {p, p}}, Values: [2]int32{20, 22}}
		gotEnvelope, err := b.Echo_envelope(envelope)
		check("unpacked parent and packed nested arrays", gotEnvelope, BindingsRecord1{Items: [2][2]BindingsRecord0{{want, want}, {want, want}}, Values: [2]int32{21, 24}}, err)
		p.Tag, p.Tail, p.Truth, p.Context = 125, 20, false, nil
		got, err = b.Echo_packed(p)
		check("false/null fields", got, BindingsRecord0{Tag: 126, Value: 22, Tail: 19, Truth: true}, err)
		got, err = b.Packed_variable(p, -8, 1.5)
		p.Tail, p.Value = 12, 22
		check("packed variadic prefix/result and C promotions", got, p, err)
		if changed, err := b.Mutate_packed(native); err != nil || changed != native {
			t.Fatal("packed pointer mutation:", changed, err)
		}
		value, err := b.Sum_packed(native)
		check("unaligned native storage", value, float64(43), err)
		if pointer, err := b.Mutate_packed(nil); pointer != nil || err != nil {
			t.Fatal("null struct pointer:", pointer, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := b.Echo_packed(BindingsRecord0{}); got != (BindingsRecord0{}) || !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("closed packed call:", got, err)
	}
}

func TestPackedBindingValidation(t *testing.T) {
	if _, err := NewBindings(nil); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(err)
	}
	var absent *Bindings
	if _, err := absent.Echo_packed(BindingsRecord0{}); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(err)
	}
	empty := dylib.New(dylib.Options{})
	defer empty.Close()
	saved := _BindingsDeclarations.Target
	defer func() { _BindingsDeclarations.Target = saved }()
	_BindingsDeclarations.Target.PointerSize = 1
	if _, err := NewBindings(empty); err == nil || !strings.Contains(err.Error(), "declaration target does not match") {
		t.Fatal("foreign target accepted:", err)
	}
}
