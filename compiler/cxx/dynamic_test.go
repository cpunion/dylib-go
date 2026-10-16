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
			// Register library-directory cleanup before the session's cleanup:
			// Windows cannot remove a DLL until its loader handle is closed.
			library := shared(t, source, optimization)
			session := dylib.New(dylib.Options{})
			t.Cleanup(func() {
				if err := session.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := session.Load(library); err != nil {
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
			call := func(t *testing.T, name string, want int32, args ...abi.Value) {
				t.Helper()
				got, err := functions[name].Call(args...)
				if err != nil || got != abi.Int32(want) {
					t.Fatalf("%s: %v, %v; want status %d", name, got, err, want)
				}
			}
			value := func(t *testing.T, kind abi.Type, initial abi.Value) *abi.NativeValue {
				t.Helper()
				v, err := abi.NewNativeValue(abi.TypeDesc{Type: kind}, initial)
				if err != nil {
					t.Fatal(err)
				}
				return v
			}
			output := func(t *testing.T, name string, kind abi.Type, status int32, args ...abi.Value) abi.Value {
				t.Helper()
				initial := abi.Zero(abi.TypeDesc{Type: kind})
				if kind == abi.I32 {
					initial = abi.Int32(1234) // Failed calls must clear their result.
				}
				out := value(t, kind, initial)
				defer out.Close()
				if err := out.WithAddress(func(address uintptr) error {
					call(t, name, status, append(args, abi.Ptr(address))...)
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
			consume := func(t *testing.T, name string, handle *abi.Value, status int32) {
				t.Helper()
				slot := value(t, abi.Pointer, *handle)
				defer slot.Close()
				if err := slot.WithAddress(func(address uintptr) error {
					call(t, name, status, abi.Ptr(address))
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				*handle, err = slot.Read()
				if err != nil || *handle != abi.Ptr(0) {
					t.Fatal("delete did not consume the handle:", *handle, err)
				}
			}
			if got := output(t, "counter_new", abi.Pointer, cxx.Exception, abi.Int32(-1)); got != abi.Ptr(0) {
				t.Fatal("throwing constructor published an object:", got)
			}
			if got := output(t, "null_new", abi.Pointer, cxx.Exception); got != abi.Ptr(0) {
				t.Fatal("null allocation published an object:", got)
			}
			counter := output(t, "counter_new", abi.Pointer, cxx.Success, abi.Int32(20))
			if counter.Bits == 0 {
				t.Fatal("missing native object")
			}
			t.Cleanup(func() { consume(t, "counter_delete", &counter, cxx.Success) })
			runtime.GC()
			if got := output(t, "counter_add", abi.I32, cxx.Success, counter, abi.Int32(22)); got != abi.Int32(42) {
				t.Fatal("integer overload:", got)
			}
			if got := output(t, "counter_add", abi.I32, cxx.Exception, counter, abi.Int32(-1)); got != abi.Int32(0) {
				t.Fatal("throwing method left a result:", got)
			}
			if got := output(t, "counter_float", abi.F64, cxx.Success, counter, abi.Float64(0.5)); got != abi.Float64(42.5) {
				t.Fatal("const floating overload:", got)
			}
			if got := output(t, "counter_positive", abi.Bool, cxx.Success, counter); got != abi.Boolean(true) {
				t.Fatal("Boolean result:", got)
			}
			if got := output(t, "counter_echo", abi.Pointer, cxx.Success, counter, counter); got != counter {
				t.Fatal("opaque native pointer:", got)
			}
			call(t, "counter_add", cxx.InvalidArgument, counter, abi.Int32(1), abi.Ptr(0))
			if got := output(t, "counter_add", abi.I32, cxx.InvalidArgument, abi.Ptr(0), abi.Int32(1)); got != abi.Int32(0) {
				t.Fatal("null receiver left a result:", got)
			}
			call(t, "counter_reset", cxx.Success, counter, abi.Int32(20))
			if got := output(t, "counter_score", abi.I32, cxx.Success, counter, abi.Int32(20)); got != abi.Int32(40) {
				t.Fatal("base virtual call:", got)
			}
			derived := output(t, "counter_derived", abi.Pointer, cxx.Success, abi.Int32(20))
			if derived.Bits == 0 {
				t.Fatal("missing adjusted native base view")
			}
			t.Cleanup(func() { consume(t, "counter_delete", &derived, cxx.Success) })
			if got := output(t, "counter_score", abi.I32, cxx.Success, derived, abi.Int32(20)); got != abi.Int32(42) {
				t.Fatal("secondary-base virtual override:", got)
			}
			consume(t, "counter_delete", &derived, cxx.Success)
			consume(t, "counter_delete", &counter, cxx.Success)
			for _, kind := range []string{"derived", "diamond"} {
				t.Run(kind+"_base_views", func(t *testing.T) {
					owner := output(t, kind+"_new", abi.Pointer, cxx.Success, abi.Int32(20))
					t.Cleanup(func() { consume(t, kind+"_delete", &owner, cxx.Success) })
					view := output(t, kind+"_counter", abi.Pointer, cxx.Success, owner)
					if view.Bits == 0 || view == owner {
						t.Fatal("missing native base adjustment:", owner, view)
					}
					if got := output(t, "counter_score", abi.I32, cxx.Success, view, abi.Int32(20)); got != abi.Int32(42) {
						t.Fatal("base virtual call:", got)
					}
					if kind == "diamond" {
						left := output(t, "diamond_left", abi.Pointer, cxx.Success, owner)
						if indirect := output(t, "left_counter", abi.Pointer, cxx.Success, left); indirect != view {
							t.Fatal("virtual diamond has distinct base views:", indirect, view)
						}
					}
					call(t, kind+"_counter", cxx.InvalidArgument, owner, abi.Ptr(0))
					// Seed a borrowed output slot to verify failed casts clear it.
					stale := value(t, abi.Pointer, view)
					t.Cleanup(func() {
						if err := stale.Close(); err != nil {
							t.Error(err)
						}
					})
					if err := stale.WithAddress(func(address uintptr) error {
						call(t, kind+"_counter", cxx.InvalidArgument, abi.Ptr(0), abi.Ptr(address))
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					cleared, err := stale.Read()
					if err != nil || cleared != abi.Ptr(0) {
						t.Fatal("failed cast left a borrowed view:", cleared, err)
					}
					if got := output(t, kind+"_counter", abi.Pointer, cxx.InvalidArgument, abi.Ptr(0)); got != abi.Ptr(0) {
						t.Fatal("null receiver left a base view:", got)
					}
					// Base views borrow the original object. They neither increase
					// its live count nor require a separate native delete.
					if got := output(t, "counter_live", abi.I32, cxx.Success); got != abi.Int32(1) {
						t.Fatal("unexpected base ownership:", got)
					}
					consume(t, kind+"_delete", &owner, cxx.Success)
					if got := output(t, "counter_live", abi.I32, cxx.Success); got != abi.Int32(0) {
						t.Fatal("owned derived object leaked:", got)
					}
				})
			}
			if got := output(t, "counter_live", abi.I32, cxx.Success); got != abi.Int32(0) {
				t.Fatal("native objects leaked:", got)
			}
			for _, scalar := range []struct {
				name string
				arg  abi.Value
			}{{"signed8", abi.Int8(-128)}, {"unsigned8", abi.Uint8(255)}, {"signed16", abi.Int16(-32768)}, {"unsigned16", abi.Uint16(65535)}, {"unsigned32", abi.Uint32(0xfedcba98)}, {"signed64", abi.Int64(-1 << 50)}, {"unsigned64", abi.Uint64(1<<63 + 99)}, {"floating32", abi.Float32(20.5)}} {
				if got := output(t, "counter_"+scalar.name, scalar.arg.Type, cxx.Success, scalar.arg); got != scalar.arg {
					t.Fatal(scalar.name, got, scalar.arg)
				}
			}
			doomed := output(t, "throwing_new", abi.Pointer, cxx.Success)
			if doomed.Bits == 0 {
				t.Fatal("missing throwing-destructor object")
			}
			consume(t, "throwing_delete", &doomed, cxx.Exception)
			consume(t, "throwing_delete", &doomed, cxx.Success)
			call(t, "counter_delete", cxx.InvalidArgument, abi.Ptr(0))
		})
	}
}
