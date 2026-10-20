//go:build cgo

package thiscall

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/compiler/clang"
)

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func TestNativeGeneratedThiscall(t *testing.T) {
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = os.Getenv("CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	names := []string{"this_state", "this_calls", "this_add", "this_mixed", "this_wide", "this_pointer", "this_void", "this_payload", "this_large", "this_factory", "this_apply", "this_forward"}

	for _, target := range []string{"i686-pc-windows-msvc", "i686-w64-windows-gnu"} {
		t.Run(target, func(t *testing.T) {
			bindingType, file := "Bindings", "bindings_windows_386.go"
			if strings.Contains(target, "-gnu") {
				bindingType, file = "GNU", "gnu_windows_386.go"
			}
			h, err := clang.Parse(context.Background(), "../../testdata/thiscall.h", clang.Options{Compiler: compiler, Target: target, Functions: names})
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := h.CgoSource("thiscall", bindingType)
			if err != nil {
				t.Fatal(err)
			}
			committed, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
				t.Fatal("stale generated thiscall bindings:", file, err)
			}

			for _, opt := range []string{"-O0", "-O2"} {
				t.Run(opt, func(t *testing.T) {
					dir := t.TempDir()
					object := filepath.Join(dir, "native.o")
					run(t, compiler, "--target="+target, "-ffreestanding", "-fno-stack-protector", opt, "-c", "../../testdata/thiscall.c", "-o", object)
					ar := os.Getenv("AR")
					if ar == "" {
						ar = "ar"
					}
					archive := filepath.Join(dir, "native.a")
					run(t, ar, "rcs", archive, object)
					inputs := []struct{ kind, path string }{{"object", object}, {"archive", archive}}
					if target == "i686-w64-windows-gnu" {
						library := filepath.Join(dir, "native.dll")
						args := []string{"--target=" + target, "-shared", object, "-o", library}
						if root := os.Getenv("DYLIB_NATIVE_SYSROOT"); root != "" {
							args = append(args, "--sysroot="+root, "--rtlib=libgcc")
						}
						run(t, compiler, args...)
						inputs = append(inputs, struct{ kind, path string }{"library", library})
					}
					for _, input := range inputs {
						t.Run(input.kind, func(t *testing.T) {
							exerciseGeneratedThiscall(t, target, input.path)
						})
					}
				})
			}
		})
	}
}

func exerciseGeneratedThiscall(t *testing.T, target, path string) {
	t.Helper()
	session := dylib.New(dylib.Options{})
	defer session.Close()
	if err := session.Load(filepath.Join(os.Getenv("SystemRoot"), "SysWOW64", "msvcrt.dll")); err != nil {
		t.Fatal(err)
	}
	if err := session.Load(path); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(target, "-msvc") {
		if got, err := Add(session); err != nil || got != 42 {
			t.Fatal("README example:", got, err)
		}
	}
	b, err := producerBindings(session, target)
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := b.This_state()
	if err != nil || receiver == nil {
		t.Fatal(err)
	}
	entry, err := b.This_factory()
	if err != nil || entry == nil {
		t.Fatal(err)
	}
	const want = 20 - 120 + 65000 + 1.25 - 2.25 + (1<<40 + 5) - 20
	for i := 0; i < 100; i++ {
		if got, err := b.This_add(receiver, 22); err != nil || got != 42 {
			t.Fatal("integer:", got, err)
		}
		if got, err := b.This_mixed(receiver, -120, 65000, 1.25, -2.25, 1<<40+5, -20); err != nil || got != want {
			t.Fatal("mixed:", got, err)
		}
		if got, err := b.This_wide(receiver, math.MaxUint64-7); err != nil || got != (math.MaxUint64-7)^uint64(20)<<48 {
			t.Fatal("wide:", got, err)
		}
		if got, err := b.This_pointer(receiver, receiver); err != nil || got != receiver {
			t.Fatal("pointer:", got, err)
		}
		if err := b.This_void(receiver, 20); err != nil {
			t.Fatal("void:", err)
		}
		if got, err := b.This_payload(receiver, BindingsRecord1{A: 3, B: 4, C: 5}, 10); err != nil || got != 42 {
			t.Fatal("record argument:", got, err)
		}
		if got, err := b.This_large(receiver, 22); err != nil || got != (BindingsRecord2{A: 20, B: 22, C: 20, D: 22}) {
			t.Fatal("hidden record result:", got, err)
		}
		if got, err := b.This_apply(entry, receiver, -120, 65000, 1.25, -2.25, 1<<40+5, -20); err != nil || got != want {
			t.Fatal("cdecl to thiscall:", got, err)
		}
		if got, err := b.This_forward(receiver, entry, -120, 65000, 1.25, -2.25, 1<<40+5, -20); err != nil || got != want {
			t.Fatal("thiscall to thiscall:", got, err)
		}
	}
	if got, err := b.This_calls(receiver); err != nil || got != 900 {
		t.Fatal("receiver changes:", got, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.This_add(receiver, 22); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("closed receiver call:", err)
	}
}

// Both producer ABIs use the same logical Go values, with independent native
// prototypes. Adapt generated Go record names only at this test boundary.
type nativeBindings interface {
	This_state() (unsafe.Pointer, error)
	This_calls(unsafe.Pointer) (int32, error)
	This_add(unsafe.Pointer, int32) (int32, error)
	This_mixed(unsafe.Pointer, int8, uint16, float32, float64, uint64, int32) (float64, error)
	This_wide(unsafe.Pointer, uint64) (uint64, error)
	This_pointer(unsafe.Pointer, unsafe.Pointer) (unsafe.Pointer, error)
	This_void(unsafe.Pointer, int32) error
	This_payload(unsafe.Pointer, BindingsRecord1, int32) (int32, error)
	This_large(unsafe.Pointer, int32) (BindingsRecord2, error)
	This_factory() (unsafe.Pointer, error)
	This_apply(unsafe.Pointer, unsafe.Pointer, int8, uint16, float32, float64, uint64, int32) (float64, error)
	This_forward(unsafe.Pointer, unsafe.Pointer, int8, uint16, float32, float64, uint64, int32) (float64, error)
}
type gnuBindings struct{ *GNU }

func (b gnuBindings) This_payload(p unsafe.Pointer, v BindingsRecord1, n int32) (int32, error) {
	return b.GNU.This_payload(p, GNURecord1(v), n)
}
func (b gnuBindings) This_large(p unsafe.Pointer, n int32) (BindingsRecord2, error) {
	v, err := b.GNU.This_large(p, n)
	return BindingsRecord2(v), err
}
func producerBindings(s *dylib.Session, target string) (nativeBindings, error) {
	if strings.Contains(target, "-gnu") {
		b, err := NewGNU(s)
		return gnuBindings{b}, err
	}
	return NewBindings(s)
}
