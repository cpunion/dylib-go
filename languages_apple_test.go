//go:build libffi && cgo

package dylib

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cpunion/dylib-go/abi"
)

func TestAppleCFacadeProducers(t *testing.T) {
	selected := selectedLanguages(t)
	if !selected["swift"] && !selected["objc"] {
		t.Skip("select swift,objc with DYLIB_TEST_LANGUAGES")
	}
	if runtime.GOOS != "darwin" {
		t.Fatal("the Apple C facade probes require macOS")
	}
	requireLanguageNative(t)
	for _, language := range []string{"swift", "objc"} {
		if !selected[language] {
			continue
		}
		t.Run(language, func(t *testing.T) {
			compiler, flags, source := "", []string(nil), ""
			if language == "swift" {
				compiler = languageTool(t, "DYLIB_SWIFTC", "swiftc")
				flags = []string{"-parse-as-library"}
				source = "testdata/applefacade/exports.swift"
			} else {
				compiler = languageTool(t, "DYLIB_OBJCXX", "clang++")
				flags = []string{"-fobjc-arc"}
				source = "testdata/applefacade/exports.mm"
			}
			for _, optimization := range []string{"-O0", "-O2"} {
				t.Run(optimization, func(t *testing.T) {
					dir := t.TempDir()
					// Images stay pinned, including when testing repeats a case.
					identity := fmt.Sprintf("%x", sha256.Sum256([]byte(dir)))[:16]
					object, library := filepath.Join(dir, "facade.o"), filepath.Join(dir, "facade.dylib")
					args := append(append([]string(nil), flags...), optimization)
					if language == "swift" {
						// Swift spells its unoptimized mode -Onone.
						if optimization == "-O0" {
							args[len(args)-1] = "-Onone"
						} else {
							args[len(args)-1] = "-O"
						}
						args = append(args, "-module-name", "DylibFacadeSwift"+identity)
						command(t, compiler, append(args, "-emit-object", source, "-o", object)...)
						command(t, compiler, append(args, "-emit-library", source, "-o", library)...)
					} else {
						// Pinned ObjC images must not register the same class twice.
						args = append(args, "-DDYLIB_FACADE_CLASS=DylibFacadeObjC"+identity)
						command(t, compiler, append(args, "-c", source, "-o", object)...)
						command(t, compiler, append(args, "-dynamiclib", "-framework", "Foundation", source, "-o", library)...)
					}
					raw := New(Options{})
					defer raw.Close()
					if err := raw.Load(object); err == nil || !strings.Contains(err.Error(), "language runtime registration") {
						t.Fatal("raw language object did not reject runtime registration:", err)
					}
					testAppleCFacade(t, library)
				})
			}
		})
	}
}

func testAppleCFacade(t *testing.T, library string) {
	t.Helper()
	// Language runtimes can retain image metadata beyond the last object. Keep
	// the OS image reference until process exit; still retire Go calls normally.
	session := New(Options{KeepLibraries: true})
	t.Cleanup(func() { session.Close() })
	load(t, session, library)
	bind := func(name string, result abi.Type, args ...abi.Type) *Function {
		t.Helper()
		f, err := session.Bind(name, abi.Signature{Result: result, Args: args})
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	create := bind("apple_create", abi.Pointer, abi.I32, abi.Pointer)
	retain := bind("apple_retain", abi.Pointer, abi.Pointer)
	release := bind("apple_release", abi.Void, abi.Pointer)
	add := bind("apple_add", abi.I32, abi.Pointer, abi.I32)
	live := bind("apple_live", abi.I32)
	if value, err := retain.Call(abi.Ptr(0)); err != nil || value.Bits != 0 {
		t.Fatal("null retain:", value, err)
	}
	if _, err := release.Call(abi.Ptr(0)); err != nil {
		t.Fatal("null release:", err)
	}
	if value, err := add.Call(abi.Ptr(0), abi.Int32(22)); err != nil || value != abi.Int32(-10000) {
		t.Fatal("null object:", value, err)
	}
	plain, err := create.Call(abi.Int32(20), abi.Ptr(0))
	if err != nil || plain.Bits == 0 {
		t.Fatal("create without observer:", plain, err)
	}
	if value, err := add.Call(plain, abi.Int32(22)); err != nil || value != abi.Int32(42) {
		t.Fatal("call without observer:", value, err)
	}
	if _, err := release.Call(plain); err != nil {
		t.Fatal(err)
	}
	if value, err := live.Call(); err != nil || value != abi.Int32(0) {
		t.Fatal("unobserved object release:", value, err)
	}
	var destroyed atomic.Int32
	callback, err := abi.NewCallback(abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32}}, func(args []abi.Value) (abi.Value, error) {
		value := int32(args[0].Bits)
		if value == -1 {
			destroyed.Add(1)
			return abi.Int32(0), nil
		}
		return abi.Int32(value + 2), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { callback.Close() })
	var handle uintptr
	references, attempts := 0, 0
	stopFailure := errors.New("retry Apple object cleanup")
	registration, err := NewRegistration(RegistrationResources{
		Functions: []*Function{create, retain, release, add, live}, Callbacks: []*abi.Callback{callback},
	}, func(leases RegistrationLeases) error {
		attempts++
		if attempts == 1 {
			return stopFailure
		}
		for references > 0 {
			if _, err := leases.Functions[2].Call(abi.Ptr(handle)); err != nil {
				return err
			}
			references--
		}
		got, err := leases.Functions[4].Call()
		if err == nil && got != abi.Int32(0) {
			err = fmt.Errorf("Apple objects remain after release: %v", got)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { registration.Close(); registration.Close() })
	if err := registration.WithLeases(func(leases RegistrationLeases) error {
		entry, err := leases.Callbacks[0].Address()
		if err != nil {
			return err
		}
		value, err := leases.Functions[0].Call(abi.Int32(20), abi.Ptr(entry))
		if err != nil || value.Bits == 0 {
			return fmt.Errorf("create Apple object: %v, %v", value, err)
		}
		handle, references = uintptr(value.Bits), 1
		value, err = leases.Functions[1].Call(abi.Ptr(handle))
		if err != nil {
			return err
		}
		references++
		if uintptr(value.Bits) != handle {
			return fmt.Errorf("retain changed opaque handle: %v", value)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	if value, err := add.Call(abi.Ptr(handle), abi.Int32(20)); err != nil || value != abi.Int32(42) {
		t.Fatal("retained language object/Go callback:", value, err)
	}
	if _, err := release.Call(abi.Ptr(handle)); err != nil {
		t.Fatal(err)
	}
	references--
	if value, err := live.Call(); err != nil || value != abi.Int32(1) || destroyed.Load() != 0 {
		t.Fatal("retain did not preserve the object:", value, destroyed.Load(), err)
	}
	closed := make(chan error, 2)
	go func() { closed <- session.Close() }()
	go func() { closed <- callback.Close() }()
	waitSessionRetired(t, session)
	if _, err := add.Call(abi.Ptr(handle), abi.Int32(1)); !errors.Is(err, ErrClosed) {
		t.Fatal("ordinary call after retirement:", err)
	}
	if err := registration.Close(); !errors.Is(err, stopFailure) {
		t.Fatal("cleanup failure:", err)
	}
	select {
	case <-closed:
		t.Fatal("retired owner closed before Apple object cleanup")
	default:
	}
	if err := registration.Close(); err != nil || destroyed.Load() != 1 || callback.Err() != nil {
		t.Fatal("language destructor lost the Go callback:", err, destroyed.Load(), callback.Err())
	}
	for i := 0; i < 2; i++ {
		select {
		case err := <-closed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("retired Apple facade owner did not close")
		}
	}
}
