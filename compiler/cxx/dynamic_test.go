//go:build libffi && cgo && (linux || darwin || windows)

package cxx_test

import (
	"path/filepath"
	"runtime"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/cxx"
)

func TestNativeDynamicClassFacade(t *testing.T) {
	header, err := filepath.Abs("testdata/counter.hpp")
	if err != nil {
		t.Fatal(err)
	}
	facade, err := cxx.Generate(filepath.ToSlash(header), classes())
	if err != nil {
		t.Fatal(err)
	}
	source, metadata := prepare(t, facade)
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			session := dylib.New(dylib.Options{})
			t.Cleanup(func() { session.Close() })
			if err := session.Load(shared(t, source, optimization)); err != nil {
				t.Fatal(err)
			}
			functions := make(map[string]*dylib.Function)
			for _, name := range facade.Functions {
				declaration, err := metadata.ForHost(name)
				if err != nil {
					t.Fatal(err)
				}
				functions[name], err = session.Bind(declaration.Symbol, declaration.Signature)
				if err != nil {
					t.Fatal(err)
				}
			}
			call := func(name string, want int32, args ...abi.Value) {
				t.Helper()
				got, err := functions[name].Call(args...)
				if err != nil || got != abi.Int32(want) {
					t.Fatalf("%s: %v, %v; want status %d", name, got, err, want)
				}
			}
			value := func(kind abi.Type, initial abi.Value) *abi.NativeValue {
				t.Helper()
				v, err := abi.NewNativeValue(abi.TypeDesc{Type: kind}, initial)
				if err != nil {
					t.Fatal(err)
				}
				return v
			}
			output := func(name string, kind abi.Type, status int32, args ...abi.Value) abi.Value {
				t.Helper()
				initial := abi.Zero(abi.TypeDesc{Type: kind})
				if kind == abi.I32 {
					initial = abi.Int32(1234) // Failed calls must clear their result.
				}
				out := value(kind, initial)
				defer out.Close()
				if err := out.WithAddress(func(address uintptr) error {
					call(name, status, append(args, abi.Ptr(address))...)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				got, err := out.Read()
				if err != nil {
					t.Fatal(err)
				}
				return got
			}
			consume := func(name string, handle *abi.Value, status int32) {
				t.Helper()
				slot := value(abi.Pointer, *handle)
				defer slot.Close()
				if err := slot.WithAddress(func(address uintptr) error {
					call(name, status, abi.Ptr(address))
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				*handle, err = slot.Read()
				if err != nil || *handle != abi.Ptr(0) {
					t.Fatal("delete did not consume the handle:", *handle, err)
				}
			}
			if got := output("counter_new", abi.Pointer, cxx.Exception, abi.Int32(-1)); got != abi.Ptr(0) {
				t.Fatal("throwing constructor published an object:", got)
			}
			if got := output("null_new", abi.Pointer, cxx.Exception); got != abi.Ptr(0) {
				t.Fatal("null allocation published an object:", got)
			}
			counter := output("counter_new", abi.Pointer, cxx.Success, abi.Int32(20))
			if counter.Bits == 0 {
				t.Fatal("missing native object")
			}
			t.Cleanup(func() { consume("counter_delete", &counter, cxx.Success) })
			runtime.GC()
			if got := output("counter_add", abi.I32, cxx.Success, counter, abi.Int32(22)); got != abi.Int32(42) {
				t.Fatal("integer overload:", got)
			}
			if got := output("counter_add", abi.I32, cxx.Exception, counter, abi.Int32(-1)); got != abi.Int32(0) {
				t.Fatal("throwing method left a result:", got)
			}
			if got := output("counter_float", abi.F64, cxx.Success, counter, abi.Float64(0.5)); got != abi.Float64(42.5) {
				t.Fatal("const floating overload:", got)
			}
			if got := output("counter_positive", abi.Bool, cxx.Success, counter); got != abi.Boolean(true) {
				t.Fatal("Boolean result:", got)
			}
			if got := output("counter_echo", abi.Pointer, cxx.Success, counter, counter); got != counter {
				t.Fatal("opaque native pointer:", got)
			}
			call("counter_add", cxx.InvalidArgument, counter, abi.Int32(1), abi.Ptr(0))
			if got := output("counter_add", abi.I32, cxx.InvalidArgument, abi.Ptr(0), abi.Int32(1)); got != abi.Int32(0) {
				t.Fatal("null receiver left a result:", got)
			}
			call("counter_reset", cxx.Success, counter, abi.Int32(20))
			if got := output("counter_score", abi.I32, cxx.Success, counter, abi.Int32(20)); got != abi.Int32(40) {
				t.Fatal("base virtual call:", got)
			}
			derived := output("counter_derived", abi.Pointer, cxx.Success, abi.Int32(20))
			if derived.Bits == 0 {
				t.Fatal("missing adjusted native base view")
			}
			t.Cleanup(func() { consume("counter_delete", &derived, cxx.Success) })
			if got := output("counter_score", abi.I32, cxx.Success, derived, abi.Int32(20)); got != abi.Int32(42) {
				t.Fatal("secondary-base virtual override:", got)
			}
			consume("counter_delete", &derived, cxx.Success)
			consume("counter_delete", &counter, cxx.Success)
			if got := output("counter_live", abi.I32, cxx.Success); got != abi.Int32(0) {
				t.Fatal("native objects leaked:", got)
			}
			for _, scalar := range []struct {
				name string
				arg  abi.Value
			}{{"signed8", abi.Int8(-128)}, {"unsigned8", abi.Uint8(255)}, {"signed16", abi.Int16(-32768)}, {"unsigned16", abi.Uint16(65535)}, {"unsigned32", abi.Uint32(0xfedcba98)}, {"signed64", abi.Int64(-1 << 50)}, {"unsigned64", abi.Uint64(1<<63 + 99)}, {"floating32", abi.Float32(20.5)}} {
				if got := output("counter_"+scalar.name, scalar.arg.Type, cxx.Success, scalar.arg); got != scalar.arg {
					t.Fatal(scalar.name, got, scalar.arg)
				}
			}
			doomed := output("throwing_new", abi.Pointer, cxx.Success)
			if doomed.Bits == 0 {
				t.Fatal("missing throwing-destructor object")
			}
			consume("throwing_delete", &doomed, cxx.Exception)
			consume("throwing_delete", &doomed, cxx.Success)
			call("counter_delete", cxx.InvalidArgument, abi.Ptr(0))
		})
	}
}
