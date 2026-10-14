//go:build cgo && (linux || darwin || windows)

package typedefbitfields

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

func TestTypedefBitfieldCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_TYPEDEF_CLANG")
	if compiler == "" {
		compiler = os.Getenv("DYLIB_LLVM_CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	names := []string{"echo_low", "echo_high", "echo_low_wide", "echo_high_wide", "echo_down", "echo_shared", "echo_float", "echo_packed", "echo_pragma", "echo_maximum", "echo_choice", "echo_envelope", "float_variadic", "high_variadic", "float_factory", "forward_float", "high_pointer", "mutate_high", "read_high", "shifted_high", "echo_member", "echo_member_packed", "echo_boundary", "echo_padding", "echo_packed_parent"}
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

	fresh, err := h.CgoSource("typedefbitfields", "Bindings")
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
		t.Fatal("typedef bitfield example", got, err)
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
	low := BindingsRecord0{Low_tag: -128, Low_bits: 7, Low_tail: 127}
	v0, err := b.Echo_low(low)
	check(v0, low, err)
	high := BindingsRecord1{High_tag: -3, High_bits: 6, High_tail: 3}
	v1, err := b.Echo_high(high)
	check(v1, high, err)
	v1, err = b.Shifted_high(2, high)
	shifted := high
	shifted.High_tag += 2
	check(v1, shifted, err)
	lowWide := BindingsRecord2{Low_wide_tag: -7, Low_wide_bits: 0x7fffffff, Low_wide_tail: 42}
	v2, err := b.Echo_low_wide(lowWide)
	check(v2, lowWide, err)
	highWide := BindingsRecord3{High_wide_tag: -9, High_wide_bits: 0x4000002a, High_wide_tail: 7}
	v3, err := b.Echo_high_wide(highWide)
	check(v3, highWide, err)
	down := BindingsRecord4{Down_tag: -5, Down_bits: 7, Down_signed: -16, Down_tail: 22}
	v4, err := b.Echo_down(down)
	check(v4, down, err)
	shared := BindingsRecord5{Shared_first: 7, Shared_second: 0x1fffffff, Shared_third: 0x40000003, Shared_tail: -11}
	v5, err := b.Echo_shared(shared)
	check(v5, shared, err)
	floating := BindingsRecord6{Float_tag: -5, Float_bits: 6, Float_real: -123.75}
	v6, err := b.Echo_float(floating)
	check(v6, floating, err)
	packed := BindingsRecord7{Packed_tag: -3, Packed_bits: 0x7fffffff, Packed_tail: -123}
	v7, err := b.Echo_packed(packed)
	check(v7, packed, err)
	pragma := BindingsRecord8{Pragma_tag: -13, Pragma_bits: 0x7fffffff, Pragma_tail: 127}
	v8, err := b.Echo_pragma(pragma)
	check(v8, pragma, err)
	maximum := BindingsRecord9{Max_tag: 2, Max_bits: 7, Max_tail: 8}
	v9, err := b.Echo_maximum(maximum)
	check(v9, maximum, err)
	choice := BindingsRecord10{Choice_active: true, Choice_value: -1, Choice_tail: 42}
	v10, err := b.Echo_choice(choice)
	check(v10, choice, err)
	envelope := BindingsRecord11{Envelope_tag: -7, Envelope_items: [2]BindingsRecord1{high, {High_tag: 3, High_bits: 7, High_tail: -9}}, Envelope_low: low}
	v11, err := b.Echo_envelope(envelope)
	check(v11, envelope, err)
	variable, err := b.Float_variadic(BindingsRecord6{Float_tag: 7, Float_bits: 5, Float_real: 20}, 4, 6)
	check(variable, int32(42), err)
	variable, err = b.High_variadic(BindingsRecord3{High_wide_tag: 1, High_wide_bits: 20, High_wide_tail: 10}, 5, 6)
	check(variable, int32(42), err)
	entry, err := b.Float_factory()
	if err != nil || entry == nil {
		t.Fatal(entry, err)
	}
	v6, err = b.Forward_float(entry, floating)
	check(v6, floating, err)
	pointer, err := b.High_pointer(highWide)
	if err != nil || pointer == nil {
		t.Fatal(pointer, err)
	}
	returned, err := b.Mutate_high(pointer)
	check(returned, pointer, err)
	v3, err = b.Read_high(pointer)
	highWide.High_wide_bits++
	highWide.High_wide_tail += 2
	check(v3, highWide, err)
	returned, err = b.Mutate_high(nil)
	check(returned, unsafe.Pointer(nil), err)
	member := BindingsRecord12{Member_tag: -7, Member_bits: 6, Member_tail: 42}
	v12, err := b.Echo_member(member)
	check(v12, member, err)
	memberPacked := BindingsRecord13{Member_packed_tag: -9, Member_packed_bits: 0x7fffffff, Member_packed_tail: -123}
	v13, err := b.Echo_member_packed(memberPacked)
	check(v13, memberPacked, err)
	boundary := BindingsRecord14{Boundary_tag: -11, Boundary_bits: 7, Boundary_tail: 42}
	v14, err := b.Echo_boundary(boundary)
	check(v14, boundary, err)
	padding := BindingsRecord15{Padding_first: 7, Padding_next: 0x7fffffff, Padding_tail: -123}
	v15, err := b.Echo_padding(padding)
	check(v15, padding, err)
	packedParent := BindingsRecord16{Packed_parent_tag: -7, Packed_parent_child: packed, Packed_parent_tail: 42}
	v16, err := b.Echo_packed_parent(packedParent)
	check(v16, packedParent, err)
}
