//go:build cgo && (linux || darwin || windows)

package bitfieldrecords

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

func TestBitfieldCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	names := []string{"echo_bits", "echo_packed_bits", "echo_envelope", "bits_factory", "apply_bits", "bits_pointer", "mutate_bits", "sum_bits", "bits_variable", "echo_signed_wide", "echo_gaps", "echo_prefix", "echo_packed_gaps", "echo_gap_envelope", "gaps_pointer", "mutate_gaps", "sum_gaps", "gaps_factory", "apply_gaps", "gaps_variable", "echo_floating_gap", "echo_member_tiny", "echo_member_trailing", "echo_packed_zero", "echo_ignored_float_zero", "echo_aligned_unit", "echo_packed_aligned_unit"}
	names = append(names, "echo_pragma_bits", "echo_pragma_tiny", "echo_pragma_packed", "echo_pragma_float", "echo_pragma_bits_envelope", "pragma_bits_factory", "apply_pragma_bits", "pragma_bits_pointer", "mutate_pragma_bits", "sum_pragma_bits", "pragma_bits_variable", "apply_pragma_bits_slot")
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	h.Functions[8], err = h.Functions[8].WithTail(abi.I8, abi.F32)
	if err != nil {
		t.Fatal(err)
	}
	h.Functions[19], err = h.Functions[19].WithTail(abi.I8, abi.F32)
	if err != nil {
		t.Fatal(err)
	}
	h.Functions[37], err = h.Functions[37].WithTail(abi.I8, abi.F32)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := h.CgoSource("bitfieldrecords", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated bitfield bindings are stale")
	}
	bad, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: h.Target.Triple, Functions: []string{"echo_bits"}})
	if err != nil {
		t.Fatal(err)
	}
	field := &bad.Records[0].Bitfields[3]
	carry := byte(0)
	for i, b := range field.Mask {
		field.Mask[i] = (b << 1) | carry
		carry = b >> 7
	}
	if carry != 0 {
		field.Mask = append(field.Mask, carry)
	}
	fresh, err = bad.CgoSource("bitfieldrecords", "BadBindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err = os.ReadFile(fmt.Sprintf("mismatch_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated mask-mismatch bindings are stale")
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
			object, archive := filepath.Join(dir, "bitfields.o"), filepath.Join(dir, "bitfields.a")
			flags := []string{"--target=" + h.Target.Triple, optimization, "-fno-stack-protector"}
			if runtime.GOOS != "windows" {
				flags = append(flags, "-fPIC")
			}
			compile(t, compiler, append(flags, "-c", "testdata/exports.c", "-o", object)...)
			compile(t, archiver, "rcs", archive, object)
			library, linkFlags := filepath.Join(dir, "bitfields.so"), []string{"-shared", "-fPIC"}
			switch runtime.GOOS {
			case "darwin":
				library, linkFlags = filepath.Join(dir, "bitfields.dylib"), []string{"-dynamiclib"}
			case "windows":
				library, linkFlags = filepath.Join(dir, "bitfields.dll"), []string{"-shared", "-Wl,--export-all-symbols"}
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
		t.Fatal("bitfield example:", got, err)
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
	for _, high := range []int32{-16, -1, 0, 14} {
		got, err := b.Echo_member_tiny(BindingsRecord9{Low: 7, High: high})
		check("small member-packed signed/unsigned fields", got, BindingsRecord9{High: high + 1}, err)
	}
	for _, code := range []uint32{0, 3, 7} {
		got, err := b.Echo_member_trailing(BindingsRecord10{Code: code})
		check("small member-packed trailing zero-width boundary", got, BindingsRecord10{Code: code ^ 7}, err)
	}
	for _, code := range []uint32{0, 3, 7} {
		got, err := b.Echo_packed_zero(BindingsRecord11{Marker: -5, Code: code, Tail: 20})
		check("packed zero-width small record argument/result", got, BindingsRecord11{Marker: -4, Code: code ^ 7, Tail: 22}, err)
	}
	for _, code := range []uint32{0, 15, 31} {
		got, err := b.Echo_aligned_unit(BindingsRecord13{Aligned_marker: -5, Aligned_code: code, Aligned_tail: 20})
		check("aligned unnamed units and ignored shared alignment", got, BindingsRecord13{Aligned_marker: -4, Aligned_code: code ^ 31, Aligned_tail: 22}, err)
		packed, err := b.Echo_packed_aligned_unit(BindingsRecord14{Packed_marker: -5, Packed_code: code, Packed_tail: 20})
		check("record-packed aligned unnamed units", packed, BindingsRecord14{Packed_marker: -4, Packed_code: code ^ 31, Packed_tail: 22}, err)
	}
	floating, err := b.Echo_ignored_float_zero(BindingsRecord12{Left: 20, Right: 18})
	check("floating record with ignored MSVC zero-width alignment", floating, BindingsRecord12{Left: 21.5, Right: 20.5}, err)
	owner, err := s.Resolve("echo_bits")
	if err != nil {
		t.Fatal(err)
	}
	err = owner.WithAddress(func(uintptr) error {
		checkPragmaCalls(t, b)
		ptr, err := b.Bits_pointer()
		if err != nil || ptr == nil {
			t.Fatal(ptr, err)
		}
		const maximum = uint64(1<<40) - 1
		p := BindingsRecord0{Flags: 7, Delta: -8, Active: true, Wide: maximum, Value: 20.5, Context: ptr}
		want := BindingsRecord0{Flags: 0, Delta: -7, Active: false, Wide: maximum - 1, Value: 22, Context: ptr}
		got, err := b.Echo_bits(p)
		check("signed/unsigned/Boolean/40-bit/native address", got, want, err)
		fn, err := b.Bits_factory()
		if err != nil || fn == nil {
			t.Fatal(fn, err)
		}
		got, err = b.Apply_bits(fn, p)
		check("bitfield function pointer", got, want, err)
		packed := BindingsRecord1{A: 7, B: -16, On: true, Big: maximum, Real: 20.5}
		wantPacked := BindingsRecord1{A: 0, B: -15, On: false, Big: maximum - 1, Real: 22}
		gotPacked, err := b.Echo_packed_bits(packed)
		check("packed crossing-byte fields", gotPacked, wantPacked, err)
		for _, value := range []int64{-1 << 39, -1, 0, (1 << 39) - 2} {
			wide, err := b.Echo_signed_wide(BindingsRecord3{Value: value, All: ^uint64(0)})
			check("signed 40-bit and full 64-bit widths", wide, BindingsRecord3{Value: value + 1}, err)
		}
		gap := BindingsRecord4{Low: 31, High: -64, Enabled: true, Value: 20}
		wantGap := BindingsRecord4{Low: 0, High: -63, Enabled: false, Value: 22}
		gotGap, err := b.Echo_gaps(gap)
		check("unnamed allocation gaps and zero-width boundaries", gotGap, wantGap, err)
		prefix, err := b.Echo_prefix(BindingsRecord5{Octet: 15})
		check("leading padding without named bitfields", prefix, BindingsRecord5{Octet: 240}, err)
		packedGap := BindingsRecord6{Low: 31, High: -64, Enabled: true, Real: 20.5}
		wantPackedGap := BindingsRecord6{Low: 0, High: -63, Enabled: false, Real: 22}
		gotPackedGap, err := b.Echo_packed_gaps(packedGap)
		check("packed unnamed and zero-width fields", gotPackedGap, wantPackedGap, err)
		floatingGap, err := b.Echo_floating_gap(BindingsRecord8{First: 20, Second: 18})
		check("floating members with unnamed gaps", floatingGap, BindingsRecord8{First: 21.5, Second: 20.5}, err)
		gapEnvelope := BindingsRecord7{Items: [2]BindingsRecord4{gap, gap}, Prefix: BindingsRecord5{Octet: 15}, Packed: [2]BindingsRecord6{packedGap, packedGap}}
		gotGapEnvelope, err := b.Echo_gap_envelope(gapEnvelope)
		check("nested gap records and arrays", gotGapEnvelope, BindingsRecord7{Items: [2]BindingsRecord4{wantGap, wantGap}, Prefix: BindingsRecord5{Octet: 240}, Packed: [2]BindingsRecord6{wantPackedGap, wantPackedGap}}, err)
		gapFn, err := b.Gaps_factory()
		if err != nil || gapFn == nil {
			t.Fatal(gapFn, err)
		}
		gotGap, err = b.Apply_gaps(gapFn, gap)
		check("gap record native factory", gotGap, wantGap, err)
		gap.High = 10
		gotGap, err = b.Gaps_variable(gap, -8, 1.5)
		gap.High, gap.Value = 2, 21
		check("gap record variadic prefix/result", gotGap, gap, err)
		gapPointer, err := b.Gaps_pointer()
		if err != nil || gapPointer == nil {
			t.Fatal(gapPointer, err)
		}
		if got, err := b.Mutate_gaps(gapPointer); err != nil || got != gapPointer {
			t.Fatal(got, err)
		}
		gapSum, err := b.Sum_gaps(gapPointer)
		check("native gap pointer mutation", gapSum, uint32(22), err)
		if got, err := b.Mutate_gaps(nil); err != nil || got != nil {
			t.Fatal(got, err)
		}
		envelope := BindingsRecord2{Regular: p, Packed: [2][2]BindingsRecord1{{packed, packed}, {packed, packed}}}
		gotEnvelope, err := b.Echo_envelope(envelope)
		check("ordinary parent and nested bitfield arrays", gotEnvelope, BindingsRecord2{Regular: want, Packed: [2][2]BindingsRecord1{{wantPacked, wantPacked}, {wantPacked, wantPacked}}}, err)
		p.Flags, p.Delta, p.Active, p.Wide, p.Context = 0, 14, false, 1, nil
		got, err = b.Echo_bits(p)
		check("positive/false/zero/null boundaries", got, BindingsRecord0{Flags: 7, Delta: 15, Active: true, Wide: 0, Value: 22}, err)
		got, err = b.Bits_variable(p, -8, 1.5)
		p.Delta, p.Value = 6, 22
		check("bitfield variadic prefix and result", got, p, err)
		if pointer, err := b.Mutate_bits(ptr); err != nil || pointer != ptr {
			t.Fatal(pointer, err)
		}
		value, err := b.Sum_bits(ptr)
		check("native field mutation", value, float64(15), err)
		if pointer, err := b.Mutate_bits(nil); pointer != nil || err != nil {
			t.Fatal(pointer, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := b.Echo_bits(BindingsRecord0{}); got != (BindingsRecord0{}) || !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(got, err)
	}
}

func checkPragmaCalls(t *testing.T, b *Bindings) {
	t.Helper()
	check := func(name string, got, want any, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: %+v, %v; want %+v", name, got, err, want)
		}
	}
	ptr, err := b.Pragma_bits_pointer()
	if err != nil || ptr == nil {
		t.Fatal(ptr, err)
	}
	const maximum = uint64(1<<40) - 1
	p := BindingsRecord15{Pragma_tag: -5, Pragma_low: 31, Pragma_high: -64, Pragma_active: true, Pragma_wide: maximum, Pragma_samples: [2]float64{20.5, 18.5}, Pragma_context: ptr}
	want := BindingsRecord15{Pragma_tag: -4, Pragma_low: 0, Pragma_high: -63, Pragma_active: false, Pragma_wide: maximum - 1, Pragma_samples: [2]float64{22, 21}, Pragma_context: ptr}
	got, err := b.Echo_pragma_bits(p)
	check("pragma bitfields, zero-width boundaries and native address", got, want, err)
	entry, err := b.Pragma_bits_factory()
	if err != nil || entry == nil {
		t.Fatal(entry, err)
	}
	got, err = b.Apply_pragma_bits(entry, p)
	check("pragma bitfield native factory", got, want, err)
	got, err = b.Apply_pragma_bits_slot(BindingsRecord20{Slot_tag: 1, Entry: entry}, p)
	slotWant := want
	slotWant.Pragma_tag++
	check("pragma function-pointer field", got, slotWant, err)
	got, err = b.Apply_pragma_bits_slot(BindingsRecord20{Slot_tag: 1}, p)
	slotWant = p
	slotWant.Pragma_tag++
	check("pragma null function-pointer field", got, slotWant, err)
	for _, high := range []int32{-16, -1, 0, 14} {
		got, err := b.Echo_pragma_tiny(BindingsRecord16{Tiny_low: 7, Tiny_high: high})
		check("pragma tiny signed/unsigned return", got, BindingsRecord16{Tiny_high: high + 1}, err)
	}
	packed := BindingsRecord17{Packed_tag: -5, Packed_low: 31, Packed_high: -64, Packed_real: 20.5}
	wantPacked := BindingsRecord17{Packed_tag: -4, Packed_low: 0, Packed_high: -63, Packed_real: 22}
	gotPacked, err := b.Echo_pragma_packed(packed)
	check("pragma plus record packing and aligned unnamed bits", gotPacked, wantPacked, err)
	floating := BindingsRecord18{Float_left: 20, Float_right: 18}
	wantFloating := BindingsRecord18{Float_left: 21.5, Float_right: 20.5}
	gotFloating, err := b.Echo_pragma_float(floating)
	check("pragma floating aggregate with inactive zero-width bits", gotFloating, wantFloating, err)
	envelope := BindingsRecord19{Envelope_tag: -5, Bits: [2][2]BindingsRecord15{{p, p}, {p, p}}, Packed: [2]BindingsRecord17{packed, packed}, Tiny: BindingsRecord16{Tiny_low: 7, Tiny_high: -16}, Floating: floating}
	wantEnvelope := BindingsRecord19{Envelope_tag: -4, Bits: [2][2]BindingsRecord15{{want, want}, {want, want}}, Packed: [2]BindingsRecord17{wantPacked, wantPacked}, Tiny: BindingsRecord16{Tiny_high: -15}, Floating: wantFloating}
	gotEnvelope, err := b.Echo_pragma_bits_envelope(envelope)
	check("pragma nested arrays, large returns and restored packing", gotEnvelope, wantEnvelope, err)
	p.Pragma_high = 10
	got, err = b.Pragma_bits_variable(p, -8, 1.5)
	p.Pragma_high, p.Pragma_samples[0] = 2, 22
	check("pragma bitfield variadic prefix/result", got, p, err)
	if got, err := b.Mutate_pragma_bits(ptr); err != nil || got != ptr {
		t.Fatal(got, err)
	}
	sum, err := b.Sum_pragma_bits(ptr)
	check("pragma native pointer mutation", sum, float64(-41), err)
	if got, err := b.Mutate_pragma_bits(nil); err != nil || got != nil {
		t.Fatal(got, err)
	}
	sum, err = b.Sum_pragma_bits(nil)
	check("pragma null native pointer", sum, float64(-1), err)
	p = BindingsRecord15{Pragma_low: 0, Pragma_high: 62, Pragma_wide: 1}
	got, err = b.Echo_pragma_bits(p)
	check("pragma positive/false/zero/null boundaries", got, BindingsRecord15{Pragma_tag: 1, Pragma_low: 31, Pragma_high: 63, Pragma_active: true, Pragma_samples: [2]float64{1.5, 2.5}}, err)
}

func TestBitfieldBindingValidation(t *testing.T) {
	if _, err := NewBindings(nil); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(err)
	}
	var absent *Bindings
	if _, err := absent.Echo_bits(BindingsRecord0{}); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(err)
	}
	empty := dylib.New(dylib.Options{})
	defer empty.Close()
	// Only the claimed bit position changed. Size, alignment and byte offsets
	// still match; a byte-mask check must reject this before resolving exports.
	if _, err := NewBadBindings(empty); err == nil || !strings.Contains(err.Error(), "compiler/cgo layout mismatch") {
		t.Fatal("wrong bit position accepted:", err)
	}
	saved := _BindingsDeclarations.Target
	defer func() { _BindingsDeclarations.Target = saved }()
	_BindingsDeclarations.Target.PointerSize = 1
	if _, err := NewBindings(empty); err == nil || !strings.Contains(err.Error(), "declaration target does not match") {
		t.Fatal(err)
	}
}
