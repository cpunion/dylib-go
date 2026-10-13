//go:build cgo && (linux || darwin || windows)

package globalpacking

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

func TestGlobalPackingCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = "clang"
	}
	compiler = tool(t, compiler)
	names := []string{"echo_packed", "echo_bits", "echo_aligned", "echo_envelope", "echo_override", "echo_restored", "packed_factory", "apply_packed", "packed_pointer", "mutate_packed", "sum_packed", "packed_variable", "apply_slot", "bits_pointer", "mutate_bits", "sum_bits", "bits_variable"}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Flags: []string{"-fpack-struct=2"}, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{11, 16} {
		h.Functions[index], err = h.Functions[index].WithTail(abi.I8, abi.F32)
		if err != nil {
			t.Fatal(err)
		}
	}
	fresh, err := h.CgoSource("globalpacking", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated global packing bindings are stale")
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
			flags := []string{"--target=" + h.Target.Triple, optimization, "-fpack-struct=2", "-fno-stack-protector"}
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
		t.Fatal("global packing example", got, err)
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
	owner, err := session.Resolve("echo_packed")
	if err != nil {
		t.Fatal(err)
	}
	err = owner.WithAddress(func(uintptr) error {
		address, err := b.Packed_pointer()
		if err != nil || address == nil {
			t.Fatal("native storage", address, err)
		}
		p := BindingsRecord0{Tag: -128, Real: 20.5, Count: 20, Truth: true, Context: address}
		want := BindingsRecord0{Tag: -127, Real: 22, Count: 22, Context: address}
		got, err := b.Echo_packed(p)
		check("default packed scalars/Boolean/address", got, want, err)
		bitsAddress, err := b.Bits_pointer()
		if err != nil || bitsAddress == nil {
			t.Fatal("native bitfield storage", bitsAddress, err)
		}
		bits := BindingsRecord1{Bits_tag: -128, Low: 7, High: -15, Active: true, Wide: 0xffffffffff, Samples: [2]float64{20.5, 19.5}, Bits_context: bitsAddress}
		bitsWant := BindingsRecord1{Bits_tag: -127, Low: 4, High: -16, Wide: 0xffffffffaa, Samples: [2]float64{22, 22}, Bits_context: bitsAddress}
		bitsGot, err := b.Echo_bits(bits)
		check("default packed named/zero-width bitfields", bitsGot, bitsWant, err)
		floats := BindingsRecord2{Left: 20.5, Right: 19.5}
		floatsWant := BindingsRecord2{Left: 22, Right: 22}
		floatsGot, err := b.Echo_aligned(floats)
		check("default packing with homogeneous float alignment", floatsGot, floatsWant, err)
		envelope := BindingsRecord3{Envelope_tag: -128, Items: [2][2]BindingsRecord0{{p, p}, {p, p}}, Bits: [2]BindingsRecord1{bits, bits}, Floats: [1]BindingsRecord2{floats}, Envelope_count: 20}
		envelopeWant := BindingsRecord3{Envelope_tag: -127, Items: [2][2]BindingsRecord0{{want, want}, {want, want}}, Bits: [2]BindingsRecord1{bitsWant, bitsWant}, Floats: [1]BindingsRecord2{floatsWant}, Envelope_count: 22}
		envelopeGot, err := b.Echo_envelope(envelope)
		check("default packed nested arrays/bitfields/alignment", envelopeGot, envelopeWant, err)
		override, err := b.Echo_override(BindingsRecord4{Special_tag: -128, Special_real: 20.5, Special_count: 20})
		check("pragma override or ignored MSVC i386 cap", override, BindingsRecord4{Special_tag: -127, Special_real: 22, Special_count: 22}, err)
		restored, err := b.Echo_restored(BindingsRecord5{Restored_tag: -128, Restored_real: 20.5, Restored_count: 20})
		check("default restored after pragma pop", restored, BindingsRecord5{Restored_tag: -127, Restored_real: 22, Restored_count: 22}, err)
		entry, err := b.Packed_factory()
		if err != nil || entry == nil {
			t.Fatal("native factory", entry, err)
		}
		got, err = b.Apply_packed(entry, p)
		check("default packed native entry", got, want, err)
		slotWant := want
		slotWant.Tag++
		got, err = b.Apply_slot(BindingsRecord6{Slot_tag: 1, Entry: entry}, p)
		check("default packed function-pointer field", got, slotWant, err)
		got, err = b.Apply_slot(BindingsRecord6{}, p)
		check("null function-pointer field", got, p, err)
		got, err = b.Packed_variable(p, -8, 1.5)
		variableWant := p
		variableWant.Count -= 8
		variableWant.Real += 1.5
		check("default packed variadic prefix/result", got, variableWant, err)
		bitsGot, err = b.Bits_variable(bits, -1, 1.5)
		variableBits := bits
		variableBits.High = -16
		variableBits.Samples[0] = 22
		check("default packed bitfield variadic prefix/result", bitsGot, variableBits, err)
		plain := p
		plain.Tag = 125
		plain.Truth = false
		plain.Context = nil
		plainWant := plain
		plainWant.Tag++
		plainWant.Real += 1.5
		plainWant.Count += 2
		plainWant.Truth = true
		got, err = b.Echo_packed(plain)
		check("false Boolean and null address", got, plainWant, err)
		changed, err := b.Mutate_packed(address)
		check("default packed pointer identity", changed, address, err)
		sum, err := b.Sum_packed(address)
		check("default packed pointer mutation", sum, float64(44), err)
		changed, err = b.Mutate_bits(bitsAddress)
		check("default packed bitfield pointer identity", changed, bitsAddress, err)
		sum, err = b.Sum_bits(bitsAddress)
		check("default packed bitfield pointer mutation", sum, float64(44), err)
		changed, err = b.Mutate_packed(nil)
		if err != nil || changed != nil {
			t.Fatal("null packed pointer", changed, err)
		}
		changed, err = b.Mutate_bits(nil)
		if err != nil || changed != nil {
			t.Fatal("null bitfield pointer", changed, err)
		}
		sum, err = b.Sum_packed(nil)
		check("null packed pointer sentinel", sum, float64(-1), err)
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
	if _, err := b.Echo_packed(BindingsRecord0{}); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("retired generated binding", err)
	}
}
