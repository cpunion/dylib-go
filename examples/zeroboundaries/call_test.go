//go:build cgo && (linux || darwin || windows)

package zeroboundaries

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

func TestZeroBoundaryCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_TYPEDEF_CLANG")
	if compiler == "" {
		compiler = os.Getenv("DYLIB_LLVM_CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	names := []string{"echo_parent", "echo_float", "echo_gap", "echo_bits", "echo_trailing", "echo_leading", "echo_consecutive", "echo_nonpower", "echo_large", "echo_mixed", "echo_packed", "echo_envelope", "shifted_float", "gap_overflow", "gap_variadic", "float_factory", "forward_float", "gap_pointer", "read_gap"}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{14} {
		h.Functions[index], err = h.Functions[index].WithTail(abi.I8, abi.F32)
		if err != nil {
			t.Fatal(err)
		}
	}

	fresh, err := h.CgoSource("zeroboundaries", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated zero-boundary bindings are stale")
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
	check := func(name string, got, want any, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal(name, got, want, err)
		}
	}
	parent := BindingsRecord1{Parent_tag: -7, Parent_child: BindingsRecord0{Child_value: -123456789}, Parent_flags: 5, Parent_tail: -1234}
	v1, err := b.Echo_parent(parent)
	check("echo_parent", v1, parent, err)
	pair := BindingsRecord2{Float_first: -1.25, Float_second: 2.5}
	v2, err := b.Echo_float(pair)
	check("echo_float", v2, pair, err)
	gap := BindingsRecord3{Gap_first: 1.5, Gap_second: -2.25}
	v3, err := b.Echo_gap(gap)
	check("echo_gap", v3, gap, err)
	bits := BindingsRecord4{Bits_tag: -9, Bits_flags: 7, Bits_tail: 1234}
	v4, err := b.Echo_bits(bits)
	check("echo_bits", v4, bits, err)
	trailing := BindingsRecord5{Trailing_tag: -42}
	v5, err := b.Echo_trailing(trailing)
	check("echo_trailing", v5, trailing, err)
	leading := BindingsRecord6{Leading_tag: 42}
	v6, err := b.Echo_leading(leading)
	check("echo_leading", v6, leading, err)
	consecutive := BindingsRecord7{Consecutive_tag: -8, Consecutive_tail: -9876}
	v7, err := b.Echo_consecutive(consecutive)
	check("echo_consecutive", v7, consecutive, err)
	nonpower := BindingsRecord8{Nonpower_head: -123456789012345, Nonpower_mid: -321, Nonpower_tail: 1234}
	v8, err := b.Echo_nonpower(nonpower)
	check("echo_nonpower", v8, nonpower, err)
	large := BindingsRecord9{Large_tag: -6, Large_child: parent.Parent_child, Large_flags: 3, Large_values: [3]float64{1.25, -2.5, 3.75}}
	v9, err := b.Echo_large(large)
	check("echo_large", v9, large, err)
	mixed := BindingsRecord10{Mixed_tag: -5, Mixed_flags: 6, Mixed_tail: -4321}
	v10, err := b.Echo_mixed(mixed)
	check("echo_mixed", v10, mixed, err)
	packed := BindingsRecord11{Packed_tag: -4, Packed_flags: 5, Packed_tail: -3.5}
	v11, err := b.Echo_packed(packed)
	check("echo_packed", v11, packed, err)
	envelope := BindingsRecord12{Envelope_gap: gap, Envelope_pair: nonpower, Envelope_items: [2]BindingsRecord5{trailing, {Trailing_tag: 17}}, Envelope_check: -987654321}
	v12, err := b.Echo_envelope(envelope)
	check("echo_envelope", v12, envelope, err)
	v2, err = b.Shifted_float(3, pair)
	check("shifted_float", v2, BindingsRecord2{Float_first: 1.75, Float_second: 2.5}, err)
	number, err := b.Gap_overflow(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord3{Gap_first: 1, Gap_second: 2}, 3)
	check("gap_overflow", number, float64(42), err)
	number, err = b.Gap_variadic(1, 2, 3, 4, 5, 6, 7, 8, BindingsRecord3{Gap_first: 1, Gap_second: 2}, 1, 2)
	check("gap_variadic", number, float64(42), err)
	entry, err := b.Float_factory()
	if err != nil || entry == nil {
		t.Fatal(entry, err)
	}
	v2, err = b.Forward_float(entry, pair)
	check("forward_float", v2, pair, err)
	pointer, err := b.Gap_pointer(gap)
	if err != nil || pointer == nil {
		t.Fatal(pointer, err)
	}
	v3, err = b.Read_gap(pointer)
	check("read_gap", v3, gap, err)
	v3, err = b.Read_gap(nil)
	check("read_gap_null", v3, BindingsRecord3{}, err)
}
