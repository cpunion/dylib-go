package dylib

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cpunion/dylib-go/abi"
)

func TestNativeCallbackReentersSameFunction(t *testing.T) {
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			s := callbackLibrary(t, input)
			sig := callbackSignature(t, "func invoke_i32(unsafe.Pointer,int32,int32)int32")
			f, err := s.Bind("invoke_i32", sig)
			if err != nil {
				t.Fatal(err)
			}
			var address uintptr
			cb, err := abi.NewCallback(callbackSignature(t, "func cb(int32,int32)int32"), func(args []abi.Value) (abi.Value, error) {
				a, b := int32(args[0].Bits), int32(args[1].Bits)
				if a == 0 {
					if _, err := s.Resolve("invoke_i32"); err != nil {
						return abi.Value{}, err
					}
					if _, err := s.Bind("invoke_i32", sig); err != nil {
						return abi.Value{}, err
					}
				}
				if a < 2 {
					return f.Call(abi.Ptr(address), abi.Int32(a+1), abi.Int32(b-1))
				}
				return abi.Int32(a + b), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer cb.Close()
			lease, err := cb.Acquire()
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			address, err = lease.Address()
			if err != nil {
				t.Fatal(err)
			}
			// Three Go -> C -> callback -> Go levels reuse one bound plan.
			got, err := f.Call(abi.Ptr(address), abi.Int32(0), abi.Int32(42))
			if err != nil || got != abi.Int32(42) || cb.Err() != nil {
				t.Fatalf("nested callback: %+v %v, callback=%v", got, err, cb.Err())
			}
		})
	}
}

func TestNativeFunctionConcurrentInvocations(t *testing.T) {
	s := callbackLibrary(t, "object")
	f, err := s.Bind("invoke_i32", callbackSignature(t, "func invoke_i32(unsafe.Pointer,int32,int32)int32"))
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}, 2), make(chan struct{})
	cb, err := abi.NewCallback(callbackSignature(t, "func cb(int32,int32)int32"), func(args []abi.Value) (abi.Value, error) {
		entered <- struct{}{}
		<-release
		return abi.Int32(int32(args[0].Bits) + int32(args[1].Bits)), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cb.Close()
	lease, err := cb.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	var releaseOnce sync.Once
	var workers sync.WaitGroup
	defer func() {
		releaseOnce.Do(func() { close(release) })
		workers.Wait() // Join native calls before releasing the callback lease.
	}()
	address, err := lease.Address()
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for i := int32(0); i < 2; i++ {
		workers.Add(1)
		go func(a int32) {
			defer workers.Done()
			got, err := f.Call(abi.Ptr(address), abi.Int32(a), abi.Int32(42-a))
			if err == nil && got != abi.Int32(42) {
				err = fmt.Errorf("concurrent result: %+v", got)
			}
			results <- err
		}(i)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("native calls through one Function were serialized")
		}
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestNativeFunctionRetirementDuringCallback(t *testing.T) {
	s := callbackLibrary(t, "object")
	f, err := s.Bind("invoke_i32", callbackSignature(t, "func invoke_i32(unsafe.Pointer,int32,int32)int32"))
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var address uintptr
	cb, err := abi.NewCallback(callbackSignature(t, "func cb(int32,int32)int32"), func(args []abi.Value) (abi.Value, error) {
		close(entered)
		<-release
		if _, err := s.Resolve("invoke_i32"); !errors.Is(err, ErrClosed) {
			return abi.Value{}, fmt.Errorf("retired resolution: %v", err)
		}
		if _, err := f.Call(abi.Ptr(address), args[0], args[1]); !errors.Is(err, ErrClosed) {
			return abi.Value{}, fmt.Errorf("retired nested call: %v", err)
		}
		return abi.Int32(42), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cb.Close()
	lease, err := cb.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	address, err = lease.Address()
	if err != nil {
		t.Fatal(err)
	}
	result, closed := make(chan error, 1), make(chan error, 1)
	var workers sync.WaitGroup
	var releaseOnce sync.Once
	defer func() {
		releaseOnce.Do(func() { close(release) })
		workers.Wait()
	}()
	workers.Add(1)
	go func() {
		defer workers.Done()
		got, err := f.Call(abi.Ptr(address), abi.Int32(20), abi.Int32(22))
		if err == nil && got != abi.Int32(42) {
			err = fmt.Errorf("in-flight call changed: %+v", got)
		}
		result <- err
	}()
	<-entered
	workers.Add(1)
	go func() {
		defer workers.Done()
		closed <- s.Close()
	}()
	waitSessionRetired(t, s)
	releaseOnce.Do(func() { close(release) })
	if err := <-result; err != nil || cb.Err() != nil {
		t.Fatalf("retiring call: %v, callback=%v", err, cb.Err())
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
}

func TestNativeConcurrentAggregateCalls(t *testing.T) {
	s := nativeABILibrary(t, "testdata/pair.c", "object")
	f, err := s.Bind("mutate_pair", callbackSignature(t, "func mutate_pair(*struct{a,b int32})*struct{a,b int32}"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := int32(0); i < 8; i++ {
		wg.Add(1)
		go func(a int32) {
			defer wg.Done()
			for n := 0; n < 20; n++ {
				desc := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "a", Type: abi.TypeDesc{Type: abi.I32}}, {Name: "b", Type: abi.TypeDesc{Type: abi.I32}}}}
				v, err := abi.StructValue(desc, abi.Int32(a), abi.Int32(42-a))
				if err != nil {
					t.Error(err)
					return
				}
				result, err := f.Call(abi.AddressOf(&v))
				if err != nil || result.Pointee != &v || v.Aggregate.Fields[0] != abi.Int32(a+2) || v.Aggregate.Fields[1] != abi.Int32(40-a) {
					t.Errorf("independent temporary storage: %+v %v", v, err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestNativeFinalizerCanObserveRetiredSession(t *testing.T) {
	if !abi.Available() {
		t.Skip("requires libffi")
	}
	needNative(t)
	s := New(Options{})
	defer s.Close()
	var events []int32
	cb, err := abi.NewCallback(callbackSignature(t, "func cb(int32)"), func(args []abi.Value) (abi.Value, error) {
		if _, err := s.Resolve("register_selective"); !errors.Is(err, ErrClosed) {
			return abi.Value{}, fmt.Errorf("finalizer resolved retired code: %v", err)
		}
		events = append(events, int32(args[0].Bits))
		return abi.Value{Type: abi.Void}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cb.Close()
	lease, err := cb.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	address, err := lease.Address()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Define("record_event", address); err != nil {
		t.Fatal(err)
	}
	load(t, s, compile(t, "testdata/lifecycle_exits.c", filepath.Join(t.TempDir(), "exits.o")))
	call(t, s, "register_selective", 20, 22, 42)
	if err := s.Close(); err != nil || cb.Err() != nil {
		t.Fatalf("finalizer reentry: %v, callback=%v", err, cb.Err())
	}
	if fmt.Sprint(events) != "[13 12 14 11]" {
		t.Fatalf("finalization events: %v", events)
	}
}

func TestNativeForeignThreadCallbackReentry(t *testing.T) {
	s := callbackThreadLibrary(t, "testdata/callback_reentrant_threads.c")
	inner, err := s.Bind("invoke_i32", callbackSignature(t, "func invoke_i32(unsafe.Pointer,int32,int32)int32"))
	if err != nil {
		t.Fatal(err)
	}
	var count atomic.Int32
	leaf, err := abi.NewCallback(callbackSignature(t, "func leaf(int32,int32)int32"), func(args []abi.Value) (abi.Value, error) {
		count.Add(1)
		return abi.Int32(int32(args[0].Bits) + int32(args[1].Bits)), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer leaf.Close()
	lease, err := leaf.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	address, err := lease.Address()
	if err != nil {
		t.Fatal(err)
	}
	outer, err := abi.NewCallback(callbackSignature(t, "func outer(int32,int32)int32"), func(args []abi.Value) (abi.Value, error) {
		if _, err := s.Resolve("invoke_i32"); err != nil {
			return abi.Value{}, err
		}
		// C worker -> Go -> the same session -> C -> another Go entry.
		return inner.Call(abi.Ptr(address), args[0], args[1])
	})
	if err != nil {
		t.Fatal(err)
	}
	defer outer.Close()
	got := callbackCall(t, s, "func invoke_threads(unsafe.Pointer,int32)int32", outer, abi.Int32(4))
	if got != abi.Int32(1680) || count.Load() != 40 || outer.Err() != nil || leaf.Err() != nil {
		t.Fatalf("foreign-thread nesting: %+v, entries=%d outer=%v leaf=%v", got, count.Load(), outer.Err(), leaf.Err())
	}
}
