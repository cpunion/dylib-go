//go:build libffi && cgo && (linux || darwin || windows)

package cxx_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	dylib "github.com/ramblelabs/dylib-go"
	"github.com/ramblelabs/dylib-go/abi"
	"github.com/ramblelabs/dylib-go/compiler/cxx"
)

func TestNativeDynamicCallableFacade(t *testing.T) {
	header, err := filepath.Abs("testdata/callables.hpp")
	if err != nil {
		t.Fatal(err)
	}
	facade, err := generateCallables(filepath.ToSlash(header))
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
			output := func(name string, initial abi.Value, status int32, args ...abi.Value) abi.Value {
				t.Helper()
				v, err := abi.NewNativeValue(abi.TypeDesc{Type: initial.Type}, initial)
				if err != nil {
					t.Fatal(err)
				}
				defer v.Close()
				err = v.WithAddress(func(address uintptr) error {
					got, err := functions[name].Call(append(args, abi.Ptr(address))...)
					if err == nil && got != abi.Int32(status) {
						return fmt.Errorf("%s: status %v; want %d", name, got, status)
					}
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				got, err := v.Read()
				if err != nil {
					t.Fatal(err)
				}
				return got
			}
			for _, prefix := range []string{"function", "captured", "move_only"} {
				t.Run(prefix, func(t *testing.T) {
					owner := output(prefix+"_make", abi.Ptr(0), cxx.Success, abi.Int32(2))
					t.Cleanup(func() {
						if got := output(prefix+"_delete", owner, cxx.Success); got != abi.Ptr(0) {
							t.Error("owned callable was not consumed", got)
						}
					})
					if got := output(prefix+"_make", owner, cxx.InvalidArgument, abi.Int32(100)); got != owner {
						t.Fatal("factory overwrote a live owner", got)
					}
					for i, want := range []int32{42, 82} {
						if got := output(prefix+"_call", abi.Int32(999), cxx.Success, owner, abi.Int32(20), abi.Int32(20)); got != abi.Int32(want) {
							t.Fatalf("stateful invocation %d: %v", i, got)
						}
					}
					if prefix == "move_only" {
						if got := output("callable_live", abi.Int32(0), cxx.Success); got != abi.Int32(1) {
							t.Fatal("move-only factory did not retain exactly one state", got)
						}
					} else {
						copy := output(prefix+"_copy", abi.Ptr(0), cxx.Success, owner)
						defer output(prefix+"_delete", copy, cxx.Success)
						if got := output(prefix+"_call", abi.Int32(0), cxx.Success, owner, abi.Int32(1), abi.Int32(2)); got != abi.Int32(85) {
							t.Fatal(got)
						}
						if got := output(prefix+"_call", abi.Int32(0), cxx.Success, copy, abi.Int32(0), abi.Int32(0)); got != abi.Int32(82) {
							t.Fatal("copies share mutable scalar capture", got)
						}
					}
					if got := output(prefix+"_call", abi.Int32(999), cxx.InvalidArgument, abi.Ptr(0), abi.Int32(20), abi.Int32(22)); got != abi.Int32(0) {
						t.Fatal("null receiver left stale output", got)
					}
				})
			}
			if got := output("callable_live", abi.Int32(999), cxx.Success); got != abi.Int32(0) {
				t.Fatal("move-only state leaked", got)
			}
			t.Run("empty_function", func(t *testing.T) {
				for _, constructor := range []string{"function_empty", "function_new"} {
					args := []abi.Value{}
					if constructor == "function_new" {
						args = append(args, abi.Ptr(0))
					}
					owner := output(constructor, abi.Ptr(0), cxx.Success, args...)
					if got := output("function_call", abi.Int32(999), cxx.Exception, owner, abi.Int32(20), abi.Int32(22)); got != abi.Int32(0) {
						t.Fatal("bad_function_call left stale output", got)
					}
					output("function_delete", owner, cxx.Success)
				}
			})
			t.Run("native_function", func(t *testing.T) {
				entry := output("callable_entry", abi.Ptr(0), cxx.Success)
				owner := output("function_new", abi.Ptr(0), cxx.Success, entry)
				defer output("function_delete", owner, cxx.Success)
				if got := output("function_call", abi.Int32(0), cxx.Success, owner, abi.Int32(20), abi.Int32(22)); got != abi.Int32(42) {
					t.Fatal(got)
				}
			})
			t.Run("retained_callback", func(t *testing.T) { testCallableRetirement(t, session, functions) })
		})
	}
}

func testCallableRetirement(t *testing.T, session *dylib.Session, functions map[string]*dylib.Function) {
	t.Helper()
	owner, err := abi.NewNativeValue(abi.TypeDesc{Type: abi.Pointer}, abi.Ptr(0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Close() })
	offset := int32(2)
	callback, err := abi.NewCallback(abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}, func(args []abi.Value) (abi.Value, error) {
		return abi.Int32(int32(args[0].Bits) + int32(args[1].Bits) + offset), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { callback.Close() })
	registration, err := dylib.NewRegistration(dylib.RegistrationResources{
		Functions: []*dylib.Function{functions["function_new"], functions["function_call"], functions["function_delete"]},
		Values:    []*abi.NativeValue{owner}, Callbacks: []*abi.Callback{callback},
	}, func(leases dylib.RegistrationLeases) error {
		object, err := leases.Values[0].Read()
		if err != nil || object.Bits == 0 {
			return err
		}
		result, err := abi.NewNativeValue(abi.TypeDesc{Type: abi.I32}, abi.Int32(0))
		if err != nil {
			return err
		}
		defer result.Close()
		callErr := result.WithAddress(func(address uintptr) error {
			status, err := leases.Functions[1].Call(object, abi.Int32(20), abi.Int32(20), abi.Ptr(address))
			if err == nil && status != abi.Int32(cxx.Success) {
				return fmt.Errorf("retiring std::function status: %v", status)
			}
			return err
		})
		got, readErr := result.Read()
		if callErr == nil && readErr == nil && got != abi.Int32(42) {
			callErr = fmt.Errorf("retiring std::function lost Go capture: %v", got)
		}
		address, err := leases.Values[0].Address()
		if err != nil {
			return err
		}
		status, err := leases.Functions[2].Call(abi.Ptr(address))
		if err == nil && status != abi.Int32(cxx.Success) {
			err = fmt.Errorf("retiring std::function delete status: %v", status)
		}
		return errors.Join(callErr, readErr, err)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { registration.Close() })
	if err := registration.WithLeases(func(leases dylib.RegistrationLeases) error {
		entry, err := leases.Callbacks[0].Address()
		if err != nil {
			return err
		}
		address, err := leases.Values[0].Address()
		if err != nil {
			return err
		}
		status, err := leases.Functions[0].Call(abi.Ptr(entry), abi.Ptr(address))
		if err == nil && status != abi.Int32(cxx.Success) {
			return fmt.Errorf("retained std::function construction: %v", status)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	closed := make(chan error, 3)
	go func() { closed <- session.Close() }()
	go func() { closed <- owner.Close() }()
	go func() { closed <- callback.Close() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := session.Resolve("function_call"); errors.Is(err, dylib.ErrClosed) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session did not retire")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-closed:
		t.Fatal("registered callable resource closed early", err)
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
			t.Fatal("callable retirement did not release its resources")
		}
	}
	if err := callback.Err(); err != nil {
		t.Fatal(err)
	}
}
