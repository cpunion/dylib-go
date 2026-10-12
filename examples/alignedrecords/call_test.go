//go:build cgo && (linux || darwin || windows)

package alignedrecords

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

func TestAlignedCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	names := []string{"echo_aligned", "echo_fields", "echo_packed", "echo_bits", "echo_envelope", "aligned_factory", "apply_aligned", "aligned_pointer", "mutate_aligned", "sum_aligned", "aligned_variable", "echo_floats", "echo_field_bits", "echo_packed_fields", "aligned_address"}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	h.Functions[10], err = h.Functions[10].WithTail(abi.I8, abi.F32)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := h.CgoSource("alignedrecords", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated aligned bindings are stale")
	}
	bad, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: h.Target.Triple, Functions: []string{"echo_aligned"}})
	if err != nil {
		t.Fatal(err)
	}
	bad.Records[0].Aligned = 16
	fresh, err = bad.CgoSource("alignedrecords", "BadBindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err = os.ReadFile(fmt.Sprintf("mismatch_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated alignment-mismatch bindings are stale")
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
			object, archive := filepath.Join(dir, "aligned.o"), filepath.Join(dir, "aligned.a")
			flags := []string{"--target=" + h.Target.Triple, optimization, "-fno-stack-protector"}
			if runtime.GOOS != "windows" {
				flags = append(flags, "-fPIC")
			}
			compile(t, compiler, append(flags, "-c", "testdata/exports.c", "-o", object)...)
			compile(t, archiver, "rcs", archive, object)
			library, linkFlags := filepath.Join(dir, "aligned.so"), []string{"-shared", "-fPIC"}
			switch runtime.GOOS {
			case "darwin":
				library, linkFlags = filepath.Join(dir, "aligned.dylib"), []string{"-dynamiclib"}
			case "windows":
				library, linkFlags = filepath.Join(dir, "aligned.dll"), []string{"-shared", "-Wl,--export-all-symbols"}
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
			// Link the target-qualified producer object rather than recompiling
			// the header under a different driver ABI (MSVC versus MinGW).
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
		t.Fatal("aligned example:", got, err)
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
	owner, err := s.Resolve("echo_aligned")
	if err != nil {
		t.Fatal(err)
	}
	err = owner.WithAddress(func(uintptr) error {
		pointer, err := b.Aligned_pointer()
		if err != nil || pointer == nil {
			t.Fatal("aligned native storage:", pointer, err)
		}
		address, err := b.Aligned_address(pointer)
		if err != nil || address == 0 || address%32 != 0 {
			t.Fatal("native address does not retain 32-byte alignment:", address, err)
		}
		p := BindingsRecord0{Tag: -128, Value: 20.5, Tail: 20, Context: pointer}
		want := BindingsRecord0{Tag: -127, Value: 22, Tail: 22, Context: pointer}
		got, err := b.Echo_aligned(p)
		check("aligned value/return", got, want, err)
		fn, err := b.Aligned_factory()
		if err != nil || fn == nil {
			t.Fatal("aligned factory:", fn, err)
		}
		got, err = b.Apply_aligned(fn, p)
		check("aligned function pointer", got, want, err)
		fields := BindingsRecord1{Tag: -128, Values: [2]float64{20.5, 19.5}, Count: 20}
		wantFields := BindingsRecord1{Tag: -127, Values: [2]float64{22, 22}, Count: 22}
		gotFields, err := b.Echo_fields(fields)
		check("aligned array member", gotFields, wantFields, err)
		packed := BindingsRecord2{Tag: -128, Value: 20.5, Tail: 23}
		wantPacked := BindingsRecord2{Tag: -127, Value: 22, Tail: 22}
		gotPacked, err := b.Echo_packed(packed)
		check("packed and aligned value", gotPacked, wantPacked, err)
		bits := BindingsRecord3{Tag: 7, Delta: -16, Value: 20.5, Enabled: 1}
		wantBits := BindingsRecord3{Tag: 0, Delta: -15, Value: 22}
		gotBits, err := b.Echo_bits(bits)
		check("aligned packed bitfields and zero-width boundary", gotBits, wantBits, err)
		envelope := BindingsRecord4{Items: [2]BindingsRecord0{p, p}, Fields: [2]BindingsRecord1{fields, fields}, Packed: [2]BindingsRecord2{packed, packed}, Bits: [2]BindingsRecord3{bits, bits}, Count: 20}
		wantEnvelope := BindingsRecord4{Items: [2]BindingsRecord0{want, want}, Fields: [2]BindingsRecord1{wantFields, wantFields}, Packed: [2]BindingsRecord2{wantPacked, wantPacked}, Bits: [2]BindingsRecord3{wantBits, wantBits}, Count: 21}
		gotEnvelope, err := b.Echo_envelope(envelope)
		check("aligned nested arrays and large return", gotEnvelope, wantEnvelope, err)
		floats, err := b.Echo_floats(BindingsRecord5{First: 20.5, Second: 19.5})
		check("aligned floating aggregate", floats, BindingsRecord5{First: 22, Second: 22}, err)
		fieldBits, err := b.Echo_field_bits(BindingsRecord6{Low: 31, Value: 20, High: -64, Tag: -128})
		check("aligned named bitfield", fieldBits, BindingsRecord6{Low: 0, Value: 22, High: -63, Tag: -127}, err)
		packedFields, err := b.Echo_packed_fields(BindingsRecord7{Tag: -128, Values: [2]float64{20.5, 19.5}, Tail: 23})
		check("packed aligned field", packedFields, BindingsRecord7{Tag: -127, Values: [2]float64{22, 22}, Tail: 22}, err)
		got, err = b.Aligned_variable(p, -8, 1.5)
		p.Tail, p.Value = 12, 22
		check("aligned variadic prefix/result", got, p, err)
		if changed, err := b.Mutate_aligned(pointer); err != nil || changed != pointer {
			t.Fatal("aligned pointer mutation:", changed, err)
		}
		value, err := b.Sum_aligned(pointer)
		check("aligned pointer dereference", value, float64(46), err)
		if pointer, err := b.Mutate_aligned(nil); err != nil || pointer != nil {
			t.Fatal("null aligned pointer:", pointer, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := b.Echo_aligned(BindingsRecord0{}); got != (BindingsRecord0{}) || !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("closed aligned call:", got, err)
	}
}

func TestAlignedBindingValidation(t *testing.T) {
	if _, err := NewBindings(nil); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(err)
	}
	var absent *Bindings
	if _, err := absent.Echo_aligned(BindingsRecord0{}); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(err)
	}
	empty := dylib.New(dylib.Options{})
	defer empty.Close()
	if _, err := NewBadBindings(empty); err == nil || !strings.Contains(err.Error(), "compiler/cgo layout mismatch") {
		t.Fatal("incorrect alignment was not rejected before symbol resolution:", err)
	}
	saved := _BindingsDeclarations.Target
	defer func() { _BindingsDeclarations.Target = saved }()
	_BindingsDeclarations.Target.PointerSize = 1
	if _, err := NewBindings(empty); err == nil || !strings.Contains(err.Error(), "declaration target does not match") {
		t.Fatal("foreign target accepted:", err)
	}
}
