//go:build cgo && (linux || darwin || windows)

package anonymousrecords

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

func TestAnonymousRecordCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_TYPEDEF_CLANG")
	if compiler == "" {
		compiler = os.Getenv("DYLIB_LLVM_CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	names := []string{"echo_high", "echo_inherited", "echo_low", "echo_low_only", "echo_float", "echo_wide", "echo_wide_low", "echo_doubles", "echo_maximum", "echo_packed", "echo_pragma", "echo_member", "echo_bits", "echo_nested", "echo_array", "echo_explicit", "shifted", "overflow", "wide_overflow", "variadic", "factory", "forward", "pointer", "read_high", "alignment"}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{19} {
		h.Functions[index], err = h.Functions[index].WithTail(abi.I8, abi.F32)
		if err != nil {
			t.Fatal(err)
		}
	}

	fresh, err := h.CgoSource("anonymousrecords", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated record typedef bindings are stale")
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
	high := BindingsRecord0{High_value: -123456789}
	for _, echo := range []func(BindingsRecord0) (BindingsRecord0, error){b.Echo_high, b.Echo_inherited, b.Echo_low} {
		got, err := echo(high)
		check(got, high, err)
	}
	low := BindingsRecord1{Low_value: 987654321}
	v1, err := b.Echo_low_only(low)
	check(v1, low, err)
	scalar := BindingsRecord2{Scalar: -123.75}
	v2, err := b.Echo_float(scalar)
	check(v2, scalar, err)
	wide := BindingsRecord3{First: -0x123456789abc, Second: 234567891234, Third: -987654321234}
	for _, echo := range []func(BindingsRecord3) (BindingsRecord3, error){b.Echo_wide, b.Echo_wide_low} {
		got, err := echo(wide)
		check(got, wide, err)
	}
	doubles := BindingsRecord4{Real_first: -123.75, Real_second: 98765.125}
	v4, err := b.Echo_doubles(doubles)
	check(v4, doubles, err)
	maximum := BindingsRecord5{Omitted_value: -123456789}
	v5, err := b.Echo_maximum(maximum)
	check(v5, maximum, err)
	packed := BindingsRecord6{Packed_tag: -3, Packed_child: high, Packed_tail: 23456789}
	v6, err := b.Echo_packed(packed)
	check(v6, packed, err)
	pragma := BindingsRecord7{Pragma_tag: -5, Pragma_child: high, Pragma_flags: 7, Pragma_tail: -12345}
	v7, err := b.Echo_pragma(pragma)
	check(v7, pragma, err)
	member := BindingsRecord8{Member_tag: -7, Member_child: high, Member_tail: 9}
	v8, err := b.Echo_member(member)
	check(v8, member, err)
	bits := BindingsRecord9{Bits_tag: -9, Bits_flags: 7, Bits_tail: 11}
	v9, err := b.Echo_bits(bits)
	check(v9, bits, err)
	nested := BindingsRecord10{Nested_tag: -11, Nested_child: high, Nested_wide: wide}
	v10, err := b.Echo_nested(nested)
	check(v10, nested, err)
	array := BindingsRecord12{Array_tag: -13, Array_values: [2]BindingsRecord11{{Pair_left: -0x123456789abc, Pair_right: 987654321234}, {Pair_left: 19, Pair_right: -23}}}
	v12, err := b.Echo_array(array)
	check(v12, array, err)
	explicit := BindingsRecord13{Explicit_tag: -15, Explicit_value: -123456789}
	v13, err := b.Echo_explicit(explicit)
	check(v13, explicit, err)
	shifted, err := b.Shifted(20, BindingsRecord0{High_value: 22})
	check(shifted, BindingsRecord0{High_value: 42}, err)
	number, err := b.Overflow(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord2{Scalar: 5}, 1)
	check(number, float64(42), err)
	integer, err := b.Wide_overflow(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord3{First: 2, Second: 1, Third: 2}, 1)
	check(integer, int64(42), err)
	number, err = b.Variadic(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord2{Scalar: 2}, 1, 3)
	check(number, float64(42), err)
	entry, err := b.Factory()
	if err != nil || entry == nil {
		t.Fatal(entry, err)
	}
	got, err := b.Forward(entry, high)
	check(got, high, err)
	pointer, err := b.Pointer(high)
	if err != nil || pointer == nil {
		t.Fatal(pointer, err)
	}
	got, err = b.Read_high(pointer)
	check(got, high, err)
	alignment, err := b.Alignment(pointer)
	check(alignment, int32(0), err)
	got, err = b.Read_high(nil)
	check(got, BindingsRecord0{}, err)
}
