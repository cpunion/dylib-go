//go:build cgo && (linux || darwin || windows)

package typedefalignment

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

func TestAlignedTypedefCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_TYPEDEF_CLANG")
	if compiler == "" {
		compiler = os.Getenv("DYLIB_LLVM_CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	names := []string{"echo_low", "echo_high", "echo_down", "echo_array", "echo_float", "echo_float_high", "echo_maximum", "echo_address", "echo_enum", "echo_packed", "echo_pragma", "scalar", "low_variadic", "high_variadic", "make_address", "apply_address", "mutate_high", "high_factory", "forward_high", "high_pointer", "read_high"}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{12, 13} {
		h.Functions[index], err = h.Functions[index].WithTail(abi.I8, abi.F32)
		if err != nil {
			t.Fatal(err)
		}
	}

	fresh, err := h.CgoSource("typedefalignment", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated typedef alignment bindings are stale")
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
		t.Fatal("typedef example", got, err)
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
	low := BindingsRecord0{Low_tag: -128, Low_value: -2147483647}
	got0, err := b.Echo_low(low)
	check(got0, low, err)
	high := BindingsRecord1{High_tag: 1, High_value: 20, High_real: 16}
	got1, err := b.Echo_high(high)
	check(got1, high, err)
	down := BindingsRecord2{Down_tag: -3, Down_value: 2147483647}
	got2, err := b.Echo_down(down)
	check(got2, down, err)
	array := BindingsRecord3{Array_tag: 7, Array_items: [2][2]int32{{-123, 2147483647}, {42, -2147483647}}}
	got3, err := b.Echo_array(array)
	check(got3, array, err)
	floating := BindingsRecord4{Float_tag: -8, Float_real: 20.5}
	got4, err := b.Echo_float(floating)
	check(got4, floating, err)
	floats := BindingsRecord5{Float_left: -123.5, Float_right: 42.25}
	got5, err := b.Echo_float_high(floats)
	check(got5, floats, err)
	maximum := BindingsRecord6{Max_tag: 22, Max_value: 20}
	got6, err := b.Echo_maximum(maximum)
	check(got6, maximum, err)
	address, err := b.Make_address()
	if err != nil || address.Address_value == nil || address.Address_entry == nil {
		t.Fatal(address, err)
	}
	got7, err := b.Echo_address(address)
	check(got7, address, err)
	applied, err := b.Apply_address(address)
	check(applied, int32(42), err)
	choice := BindingsRecord8{Enum_tag: 7, Enum_value: -1}
	got8, err := b.Echo_enum(choice)
	check(got8, choice, err)
	packed := BindingsRecord9{Packed_tag: -8, Packed_value: 123, Packed_tail: 42}
	got9, err := b.Echo_packed(packed)
	check(got9, packed, err)
	pragma := BindingsRecord10{Pragma_tag: -5, Pragma_value: 42, Pragma_real: 123.75}
	got10, err := b.Echo_pragma(pragma)
	check(got10, pragma, err)
	scalar, err := b.Scalar(20, 21, address.Address_value)
	check(scalar, int32(42), err)
	variable, err := b.Low_variadic(BindingsRecord0{Low_tag: 1, Low_value: 20}, 5, 16)
	check(variable, int32(42), err)
	variable, err = b.High_variadic(high, 2, 3)
	check(variable, int32(42), err)
	entry, err := b.High_factory()
	if err != nil || entry == nil {
		t.Fatal(entry, err)
	}
	forwarded, err := b.Forward_high(entry, high)
	check(forwarded, high, err)
	// The provider owns aligned storage; no Go memory is reinterpreted.
	pointer, err := b.High_pointer(high)
	if err != nil || pointer == nil {
		t.Fatal(pointer, err)
	}
	returned, err := b.Mutate_high(pointer)
	check(returned, pointer, err)
	mutated, err := b.Read_high(pointer)
	high.High_value++
	high.High_real += 2
	check(mutated, high, err)
}
