//go:build libffi && cgo && (linux || darwin || windows)

package dylib

import (
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/internal/native"
)

func TestNativeEmulatedTLSThreadDestructorGoCallbacks(t *testing.T) {
	needNative(t)
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			object, archive, finalizer := threadDestructorInputs(t, optimization, "-DDYLIB_TLS_WORK_CALLBACK")
			for _, kind := range []string{"object", "archive"} {
				t.Run(kind, func(t *testing.T) {
					host := nativeThreadLibrary(t, "testdata/emutls_dtor_host.c")
					var count, sum, work atomic.Int32
					callback, err := abi.NewCallback(abi.Signature{Result: abi.Void, Args: []abi.Type{abi.I32}}, func(args []abi.Value) (abi.Value, error) {
						if runtime.GOOS == "windows" && runtime.GOARCH == "386" {
							t.Logf("native TLS callback event: %d", int32(args[0].Bits))
						}
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
					s := New(Options{EmulatedTLS: true})
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
