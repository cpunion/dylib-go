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
	names := []string{"echo_packed", "echo_envelope", "packed_factory", "apply_packed", "packed_pointer", "mutate_packed", "sum_packed", "packed_variable", "echo_member", "echo_members", "echo_member_aligned", "member_factory", "apply_member", "member_pointer", "mutate_member", "sum_member", "member_variable", "echo_pragma", "echo_pragma_envelope", "echo_pragma_aligned", "echo_pragma_outer", "pragma_factory", "apply_pragma", "pragma_pointer", "mutate_pragma", "sum_pragma", "pragma_variable", "apply_pragma_slot", "echo_over", "echo_over_packed", "echo_over_max", "echo_over_type", "echo_over_array", "echo_over_envelope", "echo_over_float", "echo_over_outer", "over_factory", "apply_over", "over_pointer", "mutate_over", "sum_over", "over_variable", "apply_over_slot", "echo_over_float_nested"}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	h.Functions[7], err = h.Functions[7].WithTail(abi.I8, abi.F32)
	if err != nil {
		t.Fatal(err)
	}
	h.Functions[16], err = h.Functions[16].WithTail(abi.I8, abi.F32)
	if err != nil {
		t.Fatal(err)
	}
	h.Functions[26], err = h.Functions[26].WithTail(abi.I8, abi.F32)
	if err != nil {
		t.Fatal(err)
	}
	h.Functions[41], err = h.Functions[41].WithTail(abi.I8, abi.F32)
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
		member := BindingsRecord2{Tag: -128, Values: [2]float64{20.5, 19.5}, Tail: -32767, Context: native}
		memberWant := BindingsRecord2{Tag: -127, Values: [2]float64{22, 22}, Tail: -32768, Context: native}
		memberGot, err := b.Echo_member(member)
		check("packed array/address members", memberGot, memberWant, err)
		memberEntry, err := b.Member_factory()
		if err != nil || memberEntry == nil {
			t.Fatal("packed member factory:", memberEntry, err)
		}
		memberGot, err = b.Apply_member(memberEntry, member)
		check("packed member native function pointer", memberGot, memberWant, err)
		members := BindingsRecord3{Items: [2][2]BindingsRecord2{{member, member}, {member, member}}, Count: 20}
		membersGot, err := b.Echo_members(members)
		check("nested member-packed arrays", membersGot, BindingsRecord3{Items: [2][2]BindingsRecord2{{memberWant, memberWant}, {memberWant, memberWant}}, Count: 21}, err)
		aligned, err := b.Echo_member_aligned(BindingsRecord4{Marker: -128, Samples: [2]float64{20.5, 19.5}, Counter: 20, Truth: true})
		check("member packing and alignment", aligned, BindingsRecord4{Marker: -127, Samples: [2]float64{22, 22}, Counter: 22}, err)
		memberGot, err = b.Member_variable(member, 10, 12)
		member.Values[0], member.Tail = 32.5, -32757
		check("packed member variadic prefix/result", memberGot, member, err)
		memberAddress, err := b.Member_pointer()
		if err != nil || memberAddress == nil {
			t.Fatal("native member storage:", memberAddress, err)
		}
		if got, err := b.Mutate_member(memberAddress); err != nil || got != memberAddress {
			t.Fatal("native member pointer mutation:", got, err)
		}
		memberSum, err := b.Sum_member(memberAddress)
		check("unaligned member native storage", memberSum, float64(43), err)
		if got, err := b.Mutate_member(nil); err != nil || got != nil {
			t.Fatal("null member pointer:", got, err)
		}
		pragmaAddress, err := b.Pragma_pointer()
		if err != nil || pragmaAddress == nil {
			t.Fatal("pragma native storage:", pragmaAddress, err)
		}
		pragma := BindingsRecord5{Pragma_tag: -128, Pragma_samples: [2]float64{20.5, 19.5}, Pragma_count: 20, Pragma_truth: true, Pragma_context: pragmaAddress}
		pragmaWant := BindingsRecord5{Pragma_tag: -127, Pragma_samples: [2]float64{22, 22}, Pragma_count: 21, Pragma_context: pragmaAddress}
		pragmaGot, err := b.Echo_pragma(pragma)
		check("pragma scalar/array/Boolean/address values", pragmaGot, pragmaWant, err)
		pragmaEntry, err := b.Pragma_factory()
		if err != nil || pragmaEntry == nil {
			t.Fatal("pragma factory:", pragmaEntry, err)
		}
		pragmaGot, err = b.Apply_pragma(pragmaEntry, pragma)
		check("pragma native function pointer", pragmaGot, pragmaWant, err)
		slotWant := pragmaWant
		slotWant.Pragma_tag++
		pragmaGot, err = b.Apply_pragma_slot(BindingsRecord9{Slot_tag: 1, Entry: pragmaEntry}, pragma)
		check("packed record function-pointer field", pragmaGot, slotWant, err)
		pragmaGot, err = b.Apply_pragma_slot(BindingsRecord9{}, pragma)
		check("null packed record function-pointer field", pragmaGot, pragma, err)
		pragmaEnvelope := BindingsRecord6{Envelope_tag: -5, Pragma_items: [2][2]BindingsRecord5{{pragma, pragma}, {pragma, pragma}}, Checksum: 20}
		pragmaEnvelopeWant := BindingsRecord6{Envelope_tag: -4, Pragma_items: [2][2]BindingsRecord5{{pragmaWant, pragmaWant}, {pragmaWant, pragmaWant}}, Checksum: 22}
		pragmaEnvelopeGot, err := b.Echo_pragma_envelope(pragmaEnvelope)
		check("nested pragma packing and arrays", pragmaEnvelopeGot, pragmaEnvelopeWant, err)
		pragmaAligned, err := b.Echo_pragma_aligned(BindingsRecord7{Aligned_tag: -5, Aligned_count: 20, Aligned_value: 20.5})
		check("record alignment with pragma members", pragmaAligned, BindingsRecord7{Aligned_tag: -4, Aligned_count: 22, Aligned_value: 22}, err)
		pragmaOuter, err := b.Echo_pragma_outer(BindingsRecord8{Payload: pragmaEnvelope, Natural_tail: 19.5})
		check("pragma pop restores outer natural layout", pragmaOuter, BindingsRecord8{Payload: pragmaEnvelopeWant, Natural_tail: 22}, err)
		pragmaGot, err = b.Pragma_variable(pragma, 2, 1.5)
		pragma.Pragma_count, pragma.Pragma_samples[0] = 22, 22
		check("pragma variadic prefix/result", pragmaGot, pragma, err)
		if got, err := b.Mutate_pragma(pragmaAddress); err != nil || got != pragmaAddress {
			t.Fatal("pragma pointer mutation:", got, err)
		}
		pragmaSum, err := b.Sum_pragma(pragmaAddress)
		check("pragma native storage mutation", pragmaSum, float64(45), err)
		if got, err := b.Mutate_pragma(nil); err != nil || got != nil {
			t.Fatal("null pragma pointer:", got, err)
		}
		checkOverAlignedCalls(t, b)
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
