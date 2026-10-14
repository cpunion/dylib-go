//go:build cgo && (linux || darwin || windows)

package recordtypedefs

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

func TestRecordTypedefCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_TYPEDEF_CLANG")
	if compiler == "" {
		compiler = os.Getenv("DYLIB_LLVM_CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	names := []string{"echo_plain", "echo_high", "echo_low", "echo_inherited", "echo_wide", "echo_wide_low", "echo_float", "echo_float_low", "echo_low_parent", "echo_high_parent", "echo_down_parent", "echo_wide_parent", "echo_float_parent", "echo_array_parent", "echo_packed_parent", "echo_pragma_parent", "echo_member_parent", "shifted", "overflow", "high_factory", "forward_high", "high_pointer", "read_high", "high_alignment", "wide_overflow", "alias_variadic", "echo_small_parent", "shifted_small"}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{25} {
		h.Functions[index], err = h.Functions[index].WithTail(abi.I8, abi.F32)
		if err != nil {
			t.Fatal(err)
		}
	}

	fresh, err := h.CgoSource("recordtypedefs", "Bindings")
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
	plain := BindingsRecord0{Value: -123456789}
	for _, echo := range []func(BindingsRecord0) (BindingsRecord0, error){b.Echo_plain, b.Echo_high, b.Echo_low, b.Echo_inherited} {
		got, err := echo(plain)
		check(got, plain, err)
	}
	wide := BindingsRecord1{First: -0x123456789abc, Second: 234567891234, Third: -987654321234}
	for _, echo := range []func(BindingsRecord1) (BindingsRecord1, error){b.Echo_wide, b.Echo_wide_low} {
		got, err := echo(wide)
		check(got, wide, err)
	}
	scalar := BindingsRecord2{Scalar: -123.75}
	for _, echo := range []func(BindingsRecord2) (BindingsRecord2, error){b.Echo_float, b.Echo_float_low} {
		got, err := echo(scalar)
		check(got, scalar, err)
	}
	low := BindingsRecord3{Low_tag: -3, Low_child: plain, Low_tail: 7}
	v3, err := b.Echo_low_parent(low)
	check(v3, low, err)
	high := BindingsRecord4{High_tag: -5, High_child: plain, High_tail: 9}
	v4, err := b.Echo_high_parent(high)
	check(v4, high, err)
	down := BindingsRecord5{Down_tag: -7, Down_child: plain, Down_tail: 11}
	v5, err := b.Echo_down_parent(down)
	check(v5, down, err)
	wp := BindingsRecord6{Wide_tag: -9, Wide_child: wide, Wide_tail: 13}
	v6, err := b.Echo_wide_parent(wp)
	check(v6, wp, err)
	fp := BindingsRecord7{Float_tag: -11, Float_child: scalar, Float_tail: 15}
	v7, err := b.Echo_float_parent(fp)
	check(v7, fp, err)
	ap := BindingsRecord9{Array_tag: -13, Array_children: [2]BindingsRecord8{{Left: -0x123456789abc, Right: 234567891234}, {Left: 19, Right: -23}}, Array_tail: 17}
	v9, err := b.Echo_array_parent(ap)
	check(v9, ap, err)
	pp := BindingsRecord10{Packed_tag: -15, Packed_child: plain, Packed_tail: 19}
	v10, err := b.Echo_packed_parent(pp)
	check(v10, pp, err)
	pragma := BindingsRecord11{Pragma_tag: -17, Pragma_child: plain, Pragma_tail: 21}
	v11, err := b.Echo_pragma_parent(pragma)
	check(v11, pragma, err)
	member := BindingsRecord12{Member_tag: -19, Member_child: plain, Member_tail: 23}
	v12, err := b.Echo_member_parent(member)
	check(v12, member, err)
	shifted, err := b.Shifted(20, BindingsRecord0{Value: 22})
	check(shifted, BindingsRecord0{Value: 42}, err)
	number, err := b.Overflow(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord2{Scalar: 5}, 1)
	check(number, float64(42), err)
	integer, err := b.Wide_overflow(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord1{First: 2, Second: 1, Third: 2}, 1)
	check(integer, int64(42), err)
	number, err = b.Alias_variadic(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord2{Scalar: 2}, 1, 3)
	check(number, float64(42), err)
	small := BindingsRecord13{Small_child: plain, Small_tail: -25}
	v13, err := b.Echo_small_parent(small)
	check(v13, small, err)
	v13, err = b.Shifted_small(20, BindingsRecord13{Small_child: BindingsRecord0{Value: 22}, Small_tail: 7})
	check(v13, BindingsRecord13{Small_child: BindingsRecord0{Value: 42}, Small_tail: 7}, err)
	entry, err := b.High_factory()
	if err != nil || entry == nil {
		t.Fatal(entry, err)
	}
	got, err := b.Forward_high(entry, plain)
	check(got, plain, err)
	pointer, err := b.High_pointer(plain)
	if err != nil || pointer == nil {
		t.Fatal(pointer, err)
	}
	got, err = b.Read_high(pointer)
	check(got, plain, err)
	alignment, err := b.High_alignment(pointer)
	check(alignment, int32(0), err)
	got, err = b.Read_high(nil)
	check(got, BindingsRecord0{}, err)
}
