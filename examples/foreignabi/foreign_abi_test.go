//go:build cgo && amd64 && (linux || darwin || windows)

package foreignabi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/compiler/clang"
)

func nativeCompiler(t *testing.T) string {
	t.Helper()
	cc := os.Getenv("DYLIB_LLVM_CLANG")
	if cc == "" {
		cc = os.Getenv("CLANG")
	}
	if cc == "" {
		cc = "clang"
	}
	if _, err := exec.LookPath(cc); err != nil {
		if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	return cc
}
func run(t *testing.T, name string, args ...string) {
	t.Helper()
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}
func TestNativeGeneratedAMD64ABIs(t *testing.T) {
	cc := nativeCompiler(t)
	var names []string
	for _, prefix := range []string{"ms", "sysv"} {
		for _, name := range []string{"add", "narrow", "wide", "boolean", "pointer", "mutate", "slot", "read", "mixed", "spill", "pair", "large", "factory", "apply", "forward", "apply_large"} {
			names = append(names, prefix+"_"+name)
		}
	}
	h, err := clang.Parse(context.Background(), "../../testdata/foreign_abi.h", clang.Options{Compiler: cc, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := h.CgoSource("foreignabi", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_amd64.go", runtime.GOOS))
	if err != nil || !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("stale generated ABI declarations:", err)
	}
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			dir := t.TempDir()
			object := filepath.Join(dir, "native.o")
			flags := []string{"--target=" + h.Target.Triple, opt, "-fno-stack-protector"}
			if runtime.GOOS != "windows" {
				flags = append(flags, "-fPIC")
			} else {
				flags = append(flags, "-ffreestanding")
			}
			run(t, cc, append(append([]string{}, flags...), "-c", "../../testdata/foreign_abi.c", "-o", object)...)
			ar := os.Getenv("AR")
			if ar == "" {
				ar = "ar"
			}
			archive := filepath.Join(dir, "native.a")
			run(t, ar, "rcs", archive, object)
			library, shared := filepath.Join(dir, "native.so"), "-shared"
			if runtime.GOOS == "darwin" {
				library, shared = filepath.Join(dir, "native.dylib"), "-dynamiclib"
			}
			if runtime.GOOS == "windows" {
				library = filepath.Join(dir, "native.dll")
			}
			// Shared libraries use the native linker/runtime; raw Windows objects
			// above explicitly select the MSVC target.
			link := []string{opt, shared, "../../testdata/foreign_abi.c", "-o", library}
			if runtime.GOOS != "windows" {
				link = append(link, "--target="+h.Target.Triple, "-fPIC")
			}
			run(t, cc, link...)
			for _, input := range []struct{ kind, path string }{{"object", object}, {"archive", archive}, {"library", library}} {
				t.Run(input.kind, func(t *testing.T) {
					s := dylib.New(dylib.Options{ProcessSymbols: runtime.GOOS != "windows"})
					defer s.Close()
					if runtime.GOOS == "windows" {
						if err := s.Load(filepath.Join(os.Getenv("SystemRoot"), "System32", "msvcrt.dll")); err != nil {
							t.Fatal(err)
						}
					}
					if err := s.Load(input.path); err != nil {
						t.Fatal(err)
					}
					if a, b, err := AddBoth(s); err != nil || a != 42 || b != 42 {
						t.Fatal("library example:", a, b, err)
					}
					b, err := NewBindings(s)
					if err != nil {
						t.Fatal(err)
					}
					exerciseGeneratedABIs(t, b)
					if err := s.Close(); err != nil {
						t.Fatal(err)
					}
					if _, err := b.Ms_add(20, 22); !errors.Is(err, dylib.ErrClosed) {
						t.Fatal("retired Microsoft call:", err)
					}
					if _, err := b.Sysv_add(20, 22); !errors.Is(err, dylib.ErrClosed) {
						t.Fatal("retired SysV call:", err)
					}
				})
			}
		})
	}
}
func exerciseGeneratedABIs(t *testing.T, b *Bindings) {
	t.Helper()
	for _, pointer := range []struct {
		name   string
		slot   func() (unsafe.Pointer, error)
		mutate func(unsafe.Pointer, int32) error
		read   func(unsafe.Pointer) (int32, error)
	}{{"ms", b.Ms_slot, b.Ms_mutate, b.Ms_read}, {"sysv", b.Sysv_slot, b.Sysv_mutate, b.Sysv_read}} {
		t.Run(pointer.name+"_native_pointer", func(t *testing.T) {
			address, err := pointer.slot()
			if err != nil || address == nil {
				t.Fatal(err)
			}
			if err := pointer.mutate(address, 22); err != nil {
				t.Fatal(err)
			}
			if got, err := pointer.read(address); err != nil || got != 42 {
				t.Fatal(got, err)
			}
		})
	}

	for _, calls := range []struct {
		name           string
		add            func(int32, int32) (int32, error)
		narrow         func(int8) (int8, error)
		wide           func(uint64) (uint64, error)
		boolean        func(bool) (bool, error)
		pointer        func(unsafe.Pointer) (unsafe.Pointer, error)
		mixed          func(int8, uint16, float32, float64, int64) (float64, error)
		spill          func(int32, int32, int32, int32, int32, int32, int32, int32, float64, float64, float64, float64, float64, float64, float64, float64, float64, float64) (float64, error)
		pair           func(BindingsRecord0, int32) (BindingsRecord0, error)
		large          func(BindingsRecord1, int64) (BindingsRecord1, error)
		factory        func() (unsafe.Pointer, error)
		apply, forward func(unsafe.Pointer, BindingsRecord0, int32) (BindingsRecord0, error)
	}{
		{"ms", b.Ms_add, b.Ms_narrow, b.Ms_wide, b.Ms_boolean, b.Ms_pointer, b.Ms_mixed, b.Ms_spill, b.Ms_pair, b.Ms_large, b.Ms_factory, b.Ms_apply, b.Ms_forward},
		{"sysv", b.Sysv_add, b.Sysv_narrow, b.Sysv_wide, b.Sysv_boolean, b.Sysv_pointer, b.Sysv_mixed, b.Sysv_spill, b.Sysv_pair, b.Sysv_large, b.Sysv_factory, b.Sysv_apply, b.Sysv_forward},
	} {
		t.Run(calls.name, func(t *testing.T) {
			entry, err := calls.factory()
			if err != nil || entry == nil {
				t.Fatal("factory:", err)
			}
			// The session stays open while its returned native entry is used.
			for i := 0; i < 100; i++ {
				if got, err := calls.add(-20, 62); err != nil || got != 42 {
					t.Fatal(got, err)
				}
				if got, err := calls.narrow(-120); err != nil || got != -120 {
					t.Fatal(got, err)
				}
				if got, err := calls.wide(math.MaxUint64 - 7); err != nil || got != (math.MaxUint64-7)^0x12345678fedcba98 {
					t.Fatal(got, err)
				}
				if got, err := calls.boolean(true); err != nil || got {
					t.Fatal(got, err)
				}
				if got, err := calls.pointer(entry); err != nil || got != entry {
					t.Fatal(got, err)
				}
				if got, err := calls.mixed(-120, 65000, 1.25, -2.25, 1<<40+5); err != nil || got != -120+65000+1.25-2.25+(1<<40+5) {
					t.Fatal(got, err)
				}
				if got, err := calls.spill(0, 1, 2, 3, 4, 5, 6, 7, 0.5, 1.5, 2.5, 3.5, 4.5, 5.5, 6.5, 7.5, 8.5, 9.5); err != nil || got != 78 {
					t.Fatal(got, err)
				}
				want := BindingsRecord0{A: 42, B: 42.5}
				p := BindingsRecord0{A: 20, B: 20.5}
				if got, err := calls.pair(p, 22); err != nil || got != want {
					t.Fatal(got, err)
				}
				if got, err := calls.apply(entry, p, 22); err != nil || got != want {
					t.Fatal("cdecl consumer:", got, err)
				}
				if got, err := calls.forward(entry, p, 22); err != nil || got != want {
					t.Fatal("native consumer:", got, err)
				}
				if got, err := calls.large(BindingsRecord1{A: 1, B: 2, C: 3, D: 4, E: 5}, 22); err != nil || got != (BindingsRecord1{A: 23, B: 24, C: 25, D: 26, E: 27}) {
					t.Fatal("indirect result:", got, err)
				}
			}
		})
	}
}
