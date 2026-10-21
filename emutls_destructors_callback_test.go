//go:build libffi && cgo && (linux || darwin || windows)

package dylib

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/ramblelabs/dylib-go/abi"
	"github.com/ramblelabs/dylib-go/internal/native"
)

func TestNativeEmulatedTLSThreadDestructorGoCallbacks(t *testing.T) {
	needNative(t)
	testTLSThreadDestructorGoCallbacks(t, false)
}

func TestNativeCXXTLSThreadDestructorGoCallbacks(t *testing.T) {
	needNativeCXXTLS(t)
	testTLSThreadDestructorGoCallbacks(t, true)
}

func testTLSThreadDestructorGoCallbacks(t *testing.T, nativeMode bool) {
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			object, archive, finalizer := threadDestructorInputs(t, optimization, nativeMode, "-DDYLIB_TLS_WORK_CALLBACK")
			for _, kind := range []string{"object", "archive"} {
				t.Run(kind, func(t *testing.T) {
					host := nativeThreadLibrary(t, "testdata/emutls_dtor_host.c")
					var count, sum, work atomic.Int32
					callback, err := abi.NewCallback(abi.Signature{Result: abi.Void, Args: []abi.Type{abi.I32}}, func(args []abi.Value) (abi.Value, error) {
						if args[0].Bits == 0 {
							work.Add(1)
							return abi.Value{}, nil
						}
						count.Add(1)
						sum.Add(int32(args[0].Bits))
						return abi.Value{}, nil
					})
					if err != nil {
						t.Fatal(err)
					}
					defer callback.Close()
					lease, err := callback.Acquire()
					if err != nil {
						t.Fatal(err)
					}
					defer lease.Close()
					address, err := lease.Address()
					if err != nil {
						t.Fatal(err)
					}
					s := New(tlsDestructorOptions(nativeMode))
					defer s.Close()
					if err := s.Define("record_event", address); err != nil {
						t.Fatal(err)
					}
					input := object
					if kind == "archive" {
						input = archive
					}
					load(t, s, finalizer, input)
					step, err := s.Resolve("tls_step")
					if err != nil {
						t.Fatal(err)
					}
					start, err := host.Resolve("tls_threads")
					if err != nil {
						t.Fatal(err)
					}
					runtime.GC()
					err = step.WithAddress(func(entry uintptr) error {
						return start.WithAddress(func(run uintptr) error { native.CallPointer(run, entry); return nil })
					})
					if err != nil {
						t.Fatal(err)
					}
					call(t, host, "tls_thread_result", 0, 0, 0)
					if work.Load() != 4 || count.Load() != 16 || sum.Load() != 6888 {
						t.Fatal("foreign thread-exit Go callbacks", count.Load(), sum.Load())
					}
					if err := s.Close(); err != nil {
						t.Fatal(err)
					}
					if count.Load() != 17 || sum.Load() != 7788 || callback.Err() != nil {
						t.Fatal("native cleanup callback ownership", count.Load(), sum.Load(), callback.Err())
					}
				})
			}
		})
	}
}

func TestNativeCXXTLSConstructorReentry(t *testing.T) {
	needNativeCXXTLS(t)
	testTLSConstructorReentry(t, true)
}

func testTLSConstructorReentry(t *testing.T, nativeMode bool) {
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			object, archive, finalizer := threadDestructorInputs(t, optimization, nativeMode, "-DDYLIB_TLS_DYNAMIC_INIT", "-DDYLIB_TLS_WORK_CALLBACK")
			for _, kind := range []string{"object", "archive"} {
				t.Run(kind, func(t *testing.T) {
					host := nativeThreadLibrary(t, "testdata/emutls_dtor_host.c")
					s := New(tlsDestructorOptions(nativeMode))
					var count, sum, work, probes atomic.Int32
					callback, err := abi.NewCallback(abi.Signature{Result: abi.Void, Args: []abi.Type{abi.I32}}, func(args []abi.Value) (abi.Value, error) {
						value := int32(args[0].Bits)
						if value == 0 {
							work.Add(1)
						} else {
							count.Add(1)
							sum.Add(value)
						}
						if value == 1007 {
							marker, err := callInt32(s, "tls_guard_probe", 0, 0)
							if err != nil {
								return abi.Value{}, err
							}
							// Constant marker access precedes the first dynamic object
							// access. Compiler guards make this reentrant read safe.
							if marker != 4 {
								return abi.Value{}, fmt.Errorf("reentrant initializer marker = %d", marker)
							}
							probes.Add(1)
						}
						return abi.Value{}, nil
					})
					if err != nil {
						t.Fatal(err)
					}
					defer callback.Close()
					lease, err := callback.Acquire()
					if err != nil {
						t.Fatal(err)
					}
					defer lease.Close()
					defer s.Close()
					address, err := lease.Address()
					if err != nil {
						t.Fatal(err)
					}
					if err := s.Define("record_event", address); err != nil {
						t.Fatal(err)
					}
					input := object
					if kind == "archive" {
						input = archive
					}
					load(t, s, finalizer, input)
					step, err := s.Resolve("tls_step")
					if err != nil {
						t.Fatal(err)
					}
					start, err := host.Resolve("tls_threads")
					if err != nil {
						t.Fatal(err)
					}
					if err := step.WithAddress(func(entry uintptr) error {
						return start.WithAddress(func(run uintptr) error { native.CallPointer(run, entry); return nil })
					}); err != nil {
						t.Fatal(err)
					}
					call(t, host, "tls_thread_result", 0, 0, 0)
					if work.Load() != 4 || count.Load() != 24 || sum.Load() != 14952 || probes.Load() != 4 || callback.Err() != nil {
						t.Fatal("native constructor/Go/raw reentry", work.Load(), count.Load(), sum.Load(), probes.Load(), callback.Err())
					}
					if err := s.Close(); err != nil {
						t.Fatal(err)
					}
					if count.Load() != 25 || sum.Load() != 15852 || callback.Err() != nil {
						t.Fatal("retirement callback ownership", count.Load(), sum.Load(), callback.Err())
					}
				})
			}
		})
	}
}
