//go:build libffi && cgo && (linux || darwin || windows)

package cxx_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/cxx"
)

func TestNativeDynamicFunctionFacade(t *testing.T) {
	header, err := filepath.Abs("testdata/counter.hpp")
	if err != nil {
		t.Fatal(err)
	}
	facade, err := generateFunctions(filepath.ToSlash(header))
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
			storage := func(value abi.Value) *abi.NativeValue {
				t.Helper()
				v, err := abi.NewNativeValue(abi.TypeDesc{Type: value.Type}, value)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := v.Close(); err != nil {
						t.Error(err)
					}
				})
				return v
			}
			callOutput := func(call func(...abi.Value) (abi.Value, error), initial abi.Value, status int32, args ...abi.Value) abi.Value {
				t.Helper()
				v := storage(initial)
				err := v.WithAddress(func(address uintptr) error {
					got, err := call(append(args, abi.Ptr(address))...)
					if err == nil && got != abi.Int32(status) {
						err = fmt.Errorf("status %v; want %d", got, status)
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
			output := func(name string, initial abi.Value, status int32, args ...abi.Value) abi.Value {
				t.Helper()
				return callOutput(functions[name].Call, initial, status, args...)
			}
			native := output("callback_factory", abi.Ptr(0), cxx.Success, abi.Boolean(false))
			if native.Bits == 0 {
				t.Fatal("missing native function")
			}
			if got := output("callback_apply", abi.Int32(0), cxx.Success, native, abi.Int32(20), abi.Int32(22)); got != abi.Int32(42) {
				t.Fatal(got)
			}
			if got := output("callback_factory", native, cxx.Exception, abi.Boolean(true)); got != abi.Ptr(0) {
				t.Fatal("stale function output", got)
			}
			if got := output("callback_empty", native, cxx.Success); got != abi.Ptr(0) {
				t.Fatal("null function result", got)
			}
			withCallback := func(signature abi.Signature, handler abi.CallbackFunc, use func(abi.Value)) {
				t.Helper()
				callback, err := abi.NewCallback(signature, handler)
				if err != nil {
					t.Fatal(err)
				}
				defer callback.Close()
				if err := callback.WithAddress(func(address uintptr) error { use(abi.Ptr(address)); return nil }); err != nil {
					t.Fatal(err)
				}
				if err := callback.Err(); err != nil {
					t.Fatal(err)
				}
			}
			withCallback(abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}, func([]abi.Value) (abi.Value, error) {
				return abi.Int32(-99), nil
			}, func(entry abi.Value) {
				if got := output("callback_new", abi.Ptr(0), cxx.Exception, entry); got != abi.Ptr(0) {
					t.Fatal("throwing callback constructor published an owner", got)
				}
			})
			callbackFailure := errors.New("captured handler failed")
			failed, err := abi.NewCallback(abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}, func([]abi.Value) (abi.Value, error) {
				return abi.Value{}, callbackFailure
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { failed.Close() })
			if err := failed.WithAddress(func(address uintptr) error {
				if got := output("callback_apply", abi.Int32(999), cxx.Success, abi.Ptr(address), abi.Int32(20), abi.Int32(22)); got != abi.Int32(0) {
					t.Fatal("callback error did not return zero", got)
				}
				return nil
			}); err != nil || !errors.Is(failed.Err(), callbackFailure) {
				t.Fatal("Go callback error channel", err, failed.Err())
			}
			withCallback(abi.Signature{Result: abi.F64, Args: []abi.Type{abi.I8, abi.F32, abi.U64}}, func(args []abi.Value) (abi.Value, error) {
				if args[0] != abi.Int8(-7) || args[1] != abi.Float32(8.5) || args[2] != abi.Uint64(40) {
					return abi.Value{}, fmt.Errorf("wide callback: %v", args)
				}
				return abi.Float64(42), nil
			}, func(entry abi.Value) {
				if got := output("callback_wide", abi.Float64(0), cxx.Success, entry, abi.Int8(-7), abi.Float32(8.5), abi.Uint64(40)); got != abi.Float64(42) {
					t.Fatal(got)
				}
			})
			marker := storage(abi.Int32(0))
			if err := marker.WithAddress(func(address uintptr) error {
				withCallback(abi.Signature{Result: abi.Pointer, Args: []abi.Type{abi.Pointer}}, func(args []abi.Value) (abi.Value, error) { return args[0], nil }, func(entry abi.Value) {
					if got := output("callback_echo", abi.Ptr(0), cxx.Success, entry, abi.Ptr(address)); got != abi.Ptr(address) {
						t.Fatal(got)
					}
				})
				withCallback(abi.Signature{Result: abi.Void, Args: []abi.Type{abi.I32, abi.Pointer}}, func(args []abi.Value) (abi.Value, error) {
					if args[0] != abi.Int32(42) || args[1] != abi.Ptr(address) {
						return abi.Value{}, fmt.Errorf("void callback: %v", args)
					}
					return abi.Value{Type: abi.Void}, nil
				}, func(entry abi.Value) {
					got, err := functions["callback_notify"].Call(entry, abi.Int32(42), abi.Ptr(address))
					if err != nil || got != abi.Int32(cxx.Success) {
						t.Fatal(got, err)
					}
				})
				return nil
			}); err != nil {
				t.Fatal(err)
			}

			// A constructor retains a captured entry. Reentry uses the same leased
			// method, including while session/storage/callback Close are waiting.
			owner := storage(abi.Ptr(0))
			var retained dylib.RegistrationLeases
			offset := int32(2)
			callback, err := abi.NewCallback(abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}, func(args []abi.Value) (abi.Value, error) {
				a, b := int32(args[0].Bits), int32(args[1].Bits)
				if a != 20 {
					return abi.Int32(a + b + offset), nil
				}
				object, err := retained.Values[0].Read()
				if err != nil {
					return abi.Value{}, err
				}
				v, err := abi.NewNativeValue(abi.TypeDesc{Type: abi.I32}, abi.Int32(0))
				if err != nil {
					return abi.Value{}, err
				}
				defer v.Close()
				err = v.WithAddress(func(address uintptr) error {
					status, err := retained.Functions[0].Call(object, abi.Int32(10), abi.Int32(10), abi.Ptr(address))
					if err == nil && status != abi.Int32(cxx.Success) {
						return fmt.Errorf("nested status: %v", status)
					}
					return err
				})
				if err != nil {
					return abi.Value{}, err
				}
				value, err := v.Read()
				return abi.Int32(int32(value.Bits) + b), err
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { callback.Close() })
			registration, err := dylib.NewRegistration(dylib.RegistrationResources{
				Functions: []*dylib.Function{functions["callback_invoke"], functions["callback_new"], functions["callback_shutdown"], functions["callback_delete"]},
				Values:    []*abi.NativeValue{owner}, Callbacks: []*abi.Callback{callback},
			}, func(leases dylib.RegistrationLeases) error {
				object, err := leases.Values[0].Read()
				if err != nil {
					return err
				}
				if object.Bits == 0 {
					return nil
				}
				value, err := abi.NewNativeValue(abi.TypeDesc{Type: abi.I32}, abi.Int32(0))
				if err != nil {
					return err
				}
				defer value.Close()
				stopErr := value.WithAddress(func(address uintptr) error {
					status, err := leases.Functions[2].Call(object, abi.Ptr(address))
					if err == nil && status != abi.Int32(cxx.Success) {
						return fmt.Errorf("shutdown status: %v", status)
					}
					return err
				})
				got, readErr := value.Read()
				if stopErr == nil && readErr == nil && got != abi.Int32(42) {
					stopErr = fmt.Errorf("shutdown lost captured callback: %v", got)
				}
				address, err := leases.Values[0].Address()
				if err != nil {
					return err
				}
				status, err := leases.Functions[3].Call(abi.Ptr(address))
				if err == nil && status != abi.Int32(cxx.Success) {
					return fmt.Errorf("delete status: %v", status)
				}
				return errors.Join(stopErr, readErr, err)
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := registration.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := registration.WithLeases(func(leases dylib.RegistrationLeases) error {
				retained = leases
				entry, err := leases.Callbacks[0].Address()
				if err != nil {
					return err
				}
				address, err := leases.Values[0].Address()
				if err != nil {
					return err
				}
				status, err := leases.Functions[1].Call(abi.Ptr(entry), abi.Ptr(address))
				if err == nil && status != abi.Int32(cxx.Success) {
					return fmt.Errorf("constructor status: %v", status)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			runtime.GC()
			object, err := owner.Read()
			if err != nil {
				t.Fatal(err)
			}
			if got := output("callback_invoke", abi.Int32(0), cxx.Success, object, abi.Int32(20), abi.Int32(20)); got != abi.Int32(42) {
				t.Fatal("nested captured callback", got)
			}
			closed := make(chan error, 3)
			go func() { closed <- session.Close() }()
			go func() { closed <- owner.Close() }()
			go func() { closed <- callback.Close() }()
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := session.Resolve("callback_invoke"); errors.Is(err, dylib.ErrClosed) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("session did not retire")
				}
				time.Sleep(time.Millisecond)
			}
			if _, err := functions["callback_invoke"].Call(object, abi.Int32(20), abi.Int32(20), abi.Ptr(0)); !errors.Is(err, dylib.ErrClosed) {
				t.Fatal("ordinary call accepted a retiring session", err)
			}
			select {
			case err := <-closed:
				t.Fatal("an owner closed while still registered", err)
			default:
			}
			if err := registration.Close(); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				select {
				case err := <-closed:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("registered owner did not close")
				}
			}
			if err := callback.Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
