//go:build cgo && (linux || darwin || windows)

package overalignedbits

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

func TestOverAlignedBitfieldsCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	names := []string{"echo_tiny", "echo_bits", "echo_padding", "echo_inner", "echo_envelope", "echo_floating", "echo_packed", "echo_padding_float", "bits_factory", "apply_bits", "bits_pointer", "mutate_bits", "sum_bits", "bits_variable", "tiny_variable", "floating_variable", "apply_slot", "echo_tiny_tail", "tiny_tail_variable", "echo_triple", "triple_variable", "echo_single", "single_variable", "echo_bare", "echo_double", "echo_array", "echo_nested", "echo_gap_float", "gap_float_variable", "echo_gap_tail", "gap_tail_variable", "echo_gap_bits", "gap_bits_variable", "echo_gap_byte", "gap_byte_variable", "echo_gap_nested", "gap_nested_variable", "echo_gap_array", "gap_array_variable", "echo_gap_multiple", "gap_multiple_variable"}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{13, 15, 18, 20, 22, 28, 30, 32, 34, 36, 38, 40} {
		h.Functions[index], err = h.Functions[index].WithTail(abi.I8, abi.F32)
		if err != nil {
			t.Fatal(err)
		}
	}
	h.Functions[14], err = h.Functions[14].WithTail(abi.I8)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := h.CgoSource("overalignedbits", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated over-aligned bitfield bindings are stale")
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
		t.Fatal("over-aligned bitfield example", got, err)
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
			t.Fatalf("%s: %+v, %v; want %+v", name, got, err, want)
		}
	}
	owner, err := session.Resolve("echo_bits")
	if err != nil {
		t.Fatal(err)
	}
	err = owner.WithAddress(func(uintptr) error {
		address, err := b.Bits_pointer()
		if err != nil || address == nil {
			t.Fatal("native bitfield storage", address, err)
		}
		tiny := BindingsRecord0{Tiny_low: 7, Tiny_high: -15}
		tinyWant := BindingsRecord0{Tiny_low: 4, Tiny_high: -16}
		tinyGot, err := b.Echo_tiny(tiny)
		check("small aligned unit/shared ignored request", tinyGot, tinyWant, err)
		value := BindingsRecord1{Tag: -128, Low: 7, High: -15, Truth: true, Wide: 0xffffffffff, Real: 20.5, Count: 20, Context: address}
		want := BindingsRecord1{Tag: -127, Low: 4, High: -16, Wide: 0xffffffffaa, Real: 22, Count: 22, Context: address}
		got, err := b.Echo_bits(value)
		check("aligned named/zero/wide/ordinary units", got, want, err)
		padding := BindingsRecord2{Pad_high: -15, Samples: [2]float32{20.5, 19.5}, Pad_context: address}
		paddingWant := BindingsRecord2{Pad_high: -16, Samples: [2]float32{22, 22}, Pad_context: address}
		paddingGot, err := b.Echo_padding(padding)
		check("unnamed start/active/ignored boundaries", paddingGot, paddingWant, err)
		inner := BindingsRecord3{Inner_low: 7, Inner_samples: [2]float32{20.5, 19.5}}
		innerWant := BindingsRecord3{Inner_low: 4, Inner_samples: [2]float32{22, 22}}
		innerGot, err := b.Echo_inner(inner)
		check("nested aligned record", innerGot, innerWant, err)
		envelope := BindingsRecord4{Envelope_tag: -128, Items: [2][2]BindingsRecord1{{value, value}, {value, value}}, Pads: [2]BindingsRecord2{padding, padding}, Inners: [1]BindingsRecord3{inner}, Envelope_count: 20}
		envelopeWant := BindingsRecord4{Envelope_tag: -127, Items: [2][2]BindingsRecord1{{want, want}, {want, want}}, Pads: [2]BindingsRecord2{paddingWant, paddingWant}, Inners: [1]BindingsRecord3{innerWant}, Envelope_count: 22}
		envelopeGot, err := b.Echo_envelope(envelope)
		check("over-cap nested arrays", envelopeGot, envelopeWant, err)
		floats, err := b.Echo_floating(BindingsRecord5{Left: 20.5, Right: 19.5})
		check("ignored zero-width floating aggregate", floats, BindingsRecord5{Left: 22, Right: 22}, err)
		packed, err := b.Echo_packed(BindingsRecord6{Packed_tag: -128, Packed_high: -15, Packed_real: 20.5})
		check("packed aligned units", packed, BindingsRecord6{Packed_tag: -127, Packed_high: -16, Packed_real: 22}, err)
		gapFloats, err := b.Echo_padding_float(BindingsRecord7{Gap_left: 20.5, Gap_right: 19.5})
		check("padding-only allocation unit/float values", gapFloats, BindingsRecord7{Gap_left: 22, Gap_right: 22}, err)
		entry, err := b.Bits_factory()
		if err != nil || entry == nil {
			t.Fatal("native factory", entry, err)
		}
		got, err = b.Apply_bits(entry, value)
		check("native function-pointer entry", got, want, err)
		got, err = b.Apply_bits(nil, value)
		check("null native entry", got, value, err)
		slotWant := want
		slotWant.Tag++
		slotWant.Count += 2
		got, err = b.Apply_slot(BindingsRecord8{Slot_tag: 1, Entry: entry, Slot_flags: 2}, value)
		check("aligned native pointer mixed with bitfields", got, slotWant, err)
		got, err = b.Apply_slot(BindingsRecord8{}, value)
		check("null function-pointer field", got, value, err)
		got, err = b.Bits_variable(value, -8, 1.5)
		variable := value
		variable.Count -= 8
		variable.Real += 1.5
		check("aligned variadic prefix/result/promotions", got, variable, err)
		tinyGot, err = b.Tiny_variable(tiny, -1)
		check("small variadic aligned bitfield", tinyGot, BindingsRecord0{Tiny_low: 7, Tiny_high: -16}, err)
		floats, err = b.Floating_variable(BindingsRecord5{Left: 20.5, Right: 19.5}, -1, 1.5)
		check("floating variadic prefix/result", floats, BindingsRecord5{Left: 19.5, Right: 21}, err)
		tail := BindingsRecord9{Tail_low: 7, Tail_high: -15}
		tailGot, err := b.Echo_tiny_tail(tail)
		check("integer record with implicit tail padding", tailGot, BindingsRecord9{Tail_low: 4, Tail_high: -16}, err)
		tailGot, err = b.Tiny_tail_variable(tail, -1, 1.5)
		check("integer tail padding/variadic register positions", tailGot, BindingsRecord9{Tail_low: 6, Tail_high: -16}, err)
		triple := BindingsRecord10{Triple_left: 20.5, Triple_middle: 19.5, Triple_right: 18.5}
		tripleGot, err := b.Echo_triple(triple)
		check("partially occupied final eightbyte", tripleGot, BindingsRecord10{Triple_left: 22, Triple_middle: 22, Triple_right: 22}, err)
		tripleGot, err = b.Triple_variable(triple, -1, 1.5)
		check("three-float tail padding/variadic registers", tripleGot, BindingsRecord10{Triple_left: 19.5, Triple_middle: 21, Triple_right: 18.5}, err)
		single := BindingsRecord11{Single_left: 20.5}
		singleGot, err := b.Echo_single(single)
		check("single-float tail padding", singleGot, BindingsRecord11{Single_left: 22}, err)
		singleGot, err = b.Single_variable(single, -1, 1.5)
		check("single-float variadic prefix/result", singleGot, BindingsRecord11{Single_left: 21}, err)
		bareGot, err := b.Echo_bare(BindingsRecord12{Bare_left: 20.5})
		check("ordinary single-float result", bareGot, BindingsRecord12{Bare_left: 22}, err)
		doubleGot, err := b.Echo_double(BindingsRecord13{Double_left: 20.5})
		check("single-double result", doubleGot, BindingsRecord13{Double_left: 22}, err)
		arrayGot, err := b.Echo_array(BindingsRecord14{Array_values: [1]float32{20.5}})
		check("single-float array result", arrayGot, BindingsRecord14{Array_values: [1]float32{22}}, err)
		nestedGot, err := b.Echo_nested(BindingsRecord15{Nested: BindingsRecord12{Bare_left: 20.5}})
		check("nested single-float result", nestedGot, BindingsRecord15{Nested: BindingsRecord12{Bare_left: 22}}, err)
		interior := BindingsRecord16{Interior_left: 20.5, Interior_right: 19.5}
		interiorGot, err := b.Echo_gap_float(interior)
		check("interior floating gap", interiorGot, BindingsRecord16{Interior_left: 22, Interior_right: 22}, err)
		interiorGot, err = b.Gap_float_variable(interior, -1, 1.5)
		check("interior gap/variadic register positions", interiorGot, BindingsRecord16{Interior_left: 19.5, Interior_right: 21}, err)
		gapTail := BindingsRecord17{Tail_left: 20.5, Tail_right: 19.5}
		gapTailGot, err := b.Echo_gap_tail(gapTail)
		check("interior and trailing floating gaps", gapTailGot, BindingsRecord17{Tail_left: 22, Tail_right: 22}, err)
		gapTailGot, err = b.Gap_tail_variable(gapTail, -1, 1.5)
		check("interior/trailing gaps with variadic promotions", gapTailGot, BindingsRecord17{Tail_left: 19.5, Tail_right: 21}, err)
		gapBits := BindingsRecord18{Gap_low: 7, Gap_high: -15, Bits_right: 20.5}
		gapBitsGot, err := b.Echo_gap_bits(gapBits)
		check("named bitfields before interior gap", gapBitsGot, BindingsRecord18{Gap_low: 4, Gap_high: -16, Bits_right: 22}, err)
		gapBitsGot, err = b.Gap_bits_variable(gapBits, -1, 1.5)
		check("mixed integer/SSE variadic positions", gapBitsGot, BindingsRecord18{Gap_low: 6, Gap_high: -16, Bits_right: 22}, err)
		gapByte := BindingsRecord19{Gap_tag: -127, Byte_right: 20.5}
		gapByteGot, err := b.Echo_gap_byte(gapByte)
		check("byte and float in one register", gapByteGot, BindingsRecord19{Gap_tag: -126, Byte_right: 22}, err)
		gapByteGot, err = b.Gap_byte_variable(gapByte, -1, 1.5)
		check("signed byte boundary/variadic tail", gapByteGot, BindingsRecord19{Gap_tag: -128, Byte_right: 22}, err)
		gapNested := BindingsRecord20{Gap_nested: BindingsRecord12{Bare_left: 20.5}, Nested_right: 19.5}
		gapNestedGot, err := b.Echo_gap_nested(gapNested)
		check("nested value before interior gap", gapNestedGot, BindingsRecord20{Gap_nested: BindingsRecord12{Bare_left: 22}, Nested_right: 22}, err)
		gapNestedGot, err = b.Gap_nested_variable(gapNested, -1, 1.5)
		check("nested gap/variadic registers", gapNestedGot, BindingsRecord20{Gap_nested: BindingsRecord12{Bare_left: 19.5}, Nested_right: 21}, err)
		gapArray := BindingsRecord21{Gap_array: [1]float32{20.5}, Array_right: 19.5}
		gapArrayGot, err := b.Echo_gap_array(gapArray)
		check("array before interior gap", gapArrayGot, BindingsRecord21{Gap_array: [1]float32{22}, Array_right: 22}, err)
		gapArrayGot, err = b.Gap_array_variable(gapArray, -1, 1.5)
		check("array gap/variadic registers", gapArrayGot, BindingsRecord21{Gap_array: [1]float32{19.5}, Array_right: 21}, err)
		gapMultiple := BindingsRecord22{First_tag: -127, Next_code: -32767, Multi_right: 20.5}
		gapMultipleGot, err := b.Echo_gap_multiple(gapMultiple)
		check("multiple interior boundaries", gapMultipleGot, BindingsRecord22{First_tag: -126, Next_code: -32768, Multi_right: 22}, err)
		gapMultipleGot, err = b.Gap_multiple_variable(gapMultiple, -1, 1.5)
		check("nested implicit prefixes/variadic tail", gapMultipleGot, BindingsRecord22{First_tag: -128, Next_code: -32768, Multi_right: 22}, err)
		null := value
		null.Tag = 125
		null.Truth = false
		null.Context = nil
		nullWant := null
		nullWant.Tag++
		nullWant.Low ^= 3
		nullWant.High--
		nullWant.Truth = true
		nullWant.Wide ^= 0x55
		nullWant.Real += 1.5
		nullWant.Count += 2
		got, err = b.Echo_bits(null)
		check("false Boolean/null native address", got, nullWant, err)
		changed, err := b.Mutate_bits(address)
		check("native pointer identity", changed, address, err)
		sum, err := b.Sum_bits(address)
		check("native pointer mutation", sum, float64(44), err)
		changed, err = b.Mutate_bits(nil)
		if err != nil || changed != nil {
			t.Fatal("null bitfield pointer", changed, err)
		}
		sum, err = b.Sum_bits(nil)
		check("null bitfield pointer sentinel", sum, float64(-1), err)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Echo_bits(BindingsRecord1{}); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("retired generated binding", err)
	}
}
