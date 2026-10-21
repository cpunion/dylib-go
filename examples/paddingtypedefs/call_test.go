//go:build cgo && (linux || darwin || windows)

package paddingtypedefs

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
	"unsafe"

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

func TestPaddingTypedefCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_TYPEDEF_CLANG")
	if compiler == "" {
		compiler = os.Getenv("DYLIB_LLVM_CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	names := []string{"echo_low", "echo_high", "echo_low_wide", "echo_high_wide", "echo_zero", "echo_active", "echo_zero_low", "echo_shared", "echo_float", "echo_packed", "echo_pragma", "echo_maximum", "echo_choice", "echo_envelope", "echo_member", "echo_small", "echo_tail", "echo_down", "echo_consecutive", "echo_parent", "echo_maximum_zero", "echo_leading", "echo_aligned", "float_variadic", "small_variadic", "float_factory", "forward_float", "high_pointer", "mutate_high", "read_high", "shifted_high", "shifted_small", "small_overflow"}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{23, 24} {
		h.Functions[index], err = h.Functions[index].WithTail(abi.I8, abi.F32)
		if err != nil {
			t.Fatal(err)
		}
	}

	fresh, err := h.CgoSource("paddingtypedefs", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated unnamed typedef bindings are stale")
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
	low := BindingsRecord0{Low_tag: -128, Low_tail: 127}
	v0, err := b.Echo_low(low)
	check(v0, low, err)
	high := BindingsRecord1{High_tag: -3, High_tail: 3}
	v1, err := b.Echo_high(high)
	check(v1, high, err)
	shifted := high
	shifted.High_tag += 2
	v1, err = b.Shifted_high(2, high)
	check(v1, shifted, err)
	lowWide := BindingsRecord2{Low_wide_tag: -7, Low_wide_tail: 42}
	v2, err := b.Echo_low_wide(lowWide)
	check(v2, lowWide, err)
	highWide := BindingsRecord3{High_wide_tag: -9, High_wide_tail: 7}
	v3, err := b.Echo_high_wide(highWide)
	check(v3, highWide, err)
	zero := BindingsRecord4{Zero_tag: -5, Zero_tail: 22}
	v4, err := b.Echo_zero(zero)
	check(v4, zero, err)
	active := BindingsRecord5{Active_tag: -8, Active_bits: 7, Active_tail: 42}
	v5, err := b.Echo_active(active)
	check(v5, active, err)
	zeroLow := BindingsRecord6{Zero_low_tag: 7, Zero_low_tail: -9}
	v6, err := b.Echo_zero_low(zeroLow)
	check(v6, zeroLow, err)
	shared := BindingsRecord7{Shared_bits: 7, Shared_next: 0x40000003, Shared_tail: -11}
	v7, err := b.Echo_shared(shared)
	check(v7, shared, err)
	floating := BindingsRecord8{Float_tag: -5, Float_real: -123.75}
	v8, err := b.Echo_float(floating)
	check(v8, floating, err)
	packed := BindingsRecord9{Packed_tag: -3, Packed_tail: -123}
	v9, err := b.Echo_packed(packed)
	check(v9, packed, err)
	pragma := BindingsRecord10{Pragma_tag: -13, Pragma_tail: 127}
	v10, err := b.Echo_pragma(pragma)
	check(v10, pragma, err)
	maximum := BindingsRecord11{Max_tag: 2, Max_tail: 8}
	v11, err := b.Echo_maximum(maximum)
	check(v11, maximum, err)
	choice := BindingsRecord12{Choice_tail: 42}
	v12, err := b.Echo_choice(choice)
	check(v12, choice, err)
	envelope := BindingsRecord13{Envelope_tag: -7, Envelope_items: [2]BindingsRecord1{high, {High_tag: 3, High_tail: -9}}, Envelope_low: zeroLow}
	v13, err := b.Echo_envelope(envelope)
	check(v13, envelope, err)
	member := BindingsRecord14{Member_tag: -9, Member_tail: -123}
	v14, err := b.Echo_member(member)
	check(v14, member, err)
	small := BindingsRecord15{Small_value: 20.5, Small_tail: -9}
	v15, err := b.Echo_small(small)
	check(v15, small, err)
	moved := small
	moved.Small_tail += 2
	v15, err = b.Shifted_small(2, small)
	check(v15, moved, err)
	tail := BindingsRecord16{Tail_value: -123.75}
	v16, err := b.Echo_tail(tail)
	check(v16, tail, err)
	down := BindingsRecord17{Down_tag: -7, Down_tail: 42}
	v17, err := b.Echo_down(down)
	check(v17, down, err)
	consecutive := BindingsRecord18{Consecutive_bits: 7, Consecutive_tail: -123}
	v18, err := b.Echo_consecutive(consecutive)
	check(v18, consecutive, err)
	parent := BindingsRecord19{Parent_tag: -7, Parent_child: packed, Parent_tail: 42}
	v19, err := b.Echo_parent(parent)
	check(v19, parent, err)
	maximumZero := BindingsRecord20{Maximum_zero_tag: -11, Maximum_zero_tail: 127}
	v20, err := b.Echo_maximum_zero(maximumZero)
	check(v20, maximumZero, err)
	leading := BindingsRecord21{Leading_tag: -128, Leading_tail: 127}
	v21, err := b.Echo_leading(leading)
	check(v21, leading, err)
	aligned := BindingsRecord22{Aligned_tag: -11, Aligned_tail: 42}
	v22, err := b.Echo_aligned(aligned)
	check(v22, aligned, err)
	variable, err := b.Float_variadic(BindingsRecord8{Float_tag: 12, Float_real: 20}, 4, 6)
	check(variable, int32(42), err)
	variable, err = b.Small_variadic(BindingsRecord15{Small_value: 20, Small_tail: 12}, 4, 6)
	check(variable, int32(42), err)
	overflow, err := b.Small_overflow(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord15{Small_value: 20.5, Small_tail: 12}, 6.5)
	check(overflow, float64(75), err)
	entry, err := b.Float_factory()
	if err != nil || entry == nil {
		t.Fatal(entry, err)
	}
	v8, err = b.Forward_float(entry, floating)
	check(v8, floating, err)
	pointer, err := b.High_pointer(highWide)
	if err != nil || pointer == nil {
		t.Fatal(pointer, err)
	}
	returned, err := b.Mutate_high(pointer)
	check(returned, pointer, err)
	highWide.High_wide_tail += 2
	v3, err = b.Read_high(pointer)
	check(v3, highWide, err)
	returned, err = b.Mutate_high(nil)
	check(returned, unsafe.Pointer(nil), err)
}
