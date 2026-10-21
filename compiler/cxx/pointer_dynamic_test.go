//go:build libffi && cgo && (linux || darwin || windows)

package cxx_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	dylib "github.com/ramblelabs/dylib-go"
	"github.com/ramblelabs/dylib-go/abi"
	"github.com/ramblelabs/dylib-go/compiler/cxx"
)

func TestNativeDynamicPointerFacade(t *testing.T) {
	header, err := filepath.Abs("testdata/pointers.hpp")
	if err != nil {
		t.Fatal(err)
	}
	facade, err := generatePointers(filepath.ToSlash(header))
	if err != nil {
		t.Fatal(err)
	}
	source, metadata := prepare(t, facade)
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			library := shared(t, source, optimization)
			session := dylib.New(dylib.Options{})
			t.Cleanup(func() { session.Close() })
			if err := session.Load(library); err != nil {
				t.Fatal(err)
			}
			functions := map[string]*dylib.Function{}
			for _, name := range facade.Functions {
				declaration, err := metadata.ForHost(name)
				if err != nil {
					t.Fatal(err)
				}
				functions[name], err = session.Bind(name, declaration.Signature)
				if err != nil {
					t.Fatal(err)
				}
			}
			output := func(name string, kind abi.Type, args ...abi.Value) abi.Value {
				t.Helper()
				v, err := abi.NewNativeValue(abi.TypeDesc{Type: kind}, abi.Zero(abi.TypeDesc{Type: kind}))
				if err != nil {
					t.Fatal(err)
				}
				defer v.Close()
				err = v.WithAddress(func(address uintptr) error {
					status, err := functions[name].Call(append(args, abi.Ptr(address))...)
					if err == nil && status != abi.Int32(cxx.Success) {
						return fmt.Errorf("%s status: %v", name, status)
					}
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				value, err := v.Read()
				if err != nil {
					t.Fatal(err)
				}
				return value
			}
			want := func(name string, expected abi.Value, args ...abi.Value) {
				t.Helper()
				if got := output(name, expected.Type, args...); got != expected {
					t.Fatalf("%s: %v; want %v", name, got, expected)
				}
			}
			numbers := output("pointer_numbers", abi.Pointer)
			want("pointer_readonly", numbers)
			want("pointer_value", abi.Int32(20), numbers)
			want("pointer_add", abi.Int32(42), numbers, abi.Int32(22))
			want("pointer_value", abi.Int32(42), numbers)
			slot := output("pointer_slot", abi.Pointer)
			want("pointer_readonly_slot", slot)
			want("pointer_advance", abi.Int32(22), slot)
			want("pointer_read_slot", abi.Int32(22), slot)
			want("pointer_row_sum", abi.Int32(42), output("pointer_rows", abi.Pointer))
			want("pointer_parse", abi.Int32(42), output("pointer_text", abi.Pointer))
			mode := output("pointer_mode", abi.Pointer)
			want("pointer_read_mode", abi.Int32(42), mode)
			payload := output("pointer_payload", abi.Pointer)
			want("pointer_read_integer", abi.Int32(42), payload)
			want("pointer_set_decimal", abi.Float64(42.5), payload, abi.Float64(42.5))
			want("pointer_read_decimal", abi.Float64(42.5), payload)
			want("pointer_opaque", payload, payload)
			want("pointer_empty", abi.Ptr(0))
			want("pointer_value", abi.Int32(-1), abi.Ptr(0))
			want("pointer_opaque", abi.Ptr(0), abi.Ptr(0))

			// A borrowed static address remains valid while a provider lease
			// keeps the library alive, including after session retirement.
			lease, err := functions["pointer_read_mode"].Acquire()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { lease.Close() })
			closed := make(chan error, 1)
			go func() { closed <- session.Close() }()
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := session.Resolve("pointer_read_mode"); errors.Is(err, dylib.ErrClosed) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("provider did not retire")
				}
				time.Sleep(time.Millisecond)
			}
			if _, err := functions["pointer_read_mode"].Call(mode, abi.Ptr(0)); !errors.Is(err, dylib.ErrClosed) {
				t.Fatal("ordinary call accepted a retired provider", err)
			}
			select {
			case err := <-closed:
				t.Fatal("borrowed address provider closed early", err)
			default:
			}
			v, err := abi.NewNativeValue(abi.TypeDesc{Type: abi.I32}, abi.Int32(0))
			if err != nil {
				t.Fatal(err)
			}
			defer v.Close()
			err = v.WithAddress(func(address uintptr) error {
				status, err := lease.Call(mode, abi.Ptr(address))
				if err == nil && status != abi.Int32(cxx.Success) {
					return fmt.Errorf("retired pointer call: %v", status)
				}
				return err
			})
			value, readErr := v.Read()
			if err != nil || readErr != nil || value != abi.Int32(42) {
				t.Fatal("borrowed address during retirement", value, err, readErr)
			}
			if err := lease.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-closed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("provider did not close after pointer lease release")
			}
		})
	}
}
