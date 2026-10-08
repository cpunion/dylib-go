package abi

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test lifetime behavior without requiring or dereferencing native addresses.
// Native concurrency/reentry is also executed by the parent package's tests.
type testCallBackend struct {
	call    func([]Value) (Value, error)
	cleanup func()
}

func (b *testCallBackend) invoke(_ uintptr, _ Signature, args []Value) (Value, error) {
	return b.call(args)
}
func (b *testCallBackend) close() {
	if b.cleanup != nil {
		b.cleanup()
	}
}

func TestCallPlanReentryAndOverlappingCalls(t *testing.T) {
	signature := Signature{Result: I32, Args: []Type{I32}}
	p := &CallPlan{signature: signature, physical: signature}
	p.backend = &testCallBackend{call: func(args []Value) (Value, error) {
		if args[0] == Int32(0) {
			return p.Call(1, Int32(42))
		}
		return args[0], nil
	}}
	if result, err := p.Call(1, Int32(0)); err != nil || result != Int32(42) {
		t.Fatalf("reentrant call: %+v %v", result, err)
	}
	p.Close()
	entered, release := make(chan struct{}, 2), make(chan struct{})
	p = &CallPlan{signature: signature, physical: signature, backend: &testCallBackend{call: func(args []Value) (Value, error) {
		entered <- struct{}{}
		<-release
		return args[0], nil
	}}}
	defer p.Close()
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			result, err := p.Call(1, Int32(42))
			if err == nil && result != Int32(42) {
				err = errors.New("concurrent argument/result storage changed")
			}
			results <- err
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("call plan serialized invocations")
		}
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestCallPlanRetirementAndCleanup(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	used, closed := make(chan error, 1), make(chan error, 2)
	var cleanups atomic.Int32
	p := &CallPlan{signature: Signature{Result: I32}, physical: Signature{Result: I32}}
	p.backend = &testCallBackend{call: func([]Value) (Value, error) {
		close(entered)
		<-release
		// A pending Close rejects callback reentry instead of blocking it.
		_, err := p.Call(1)
		if !errors.Is(err, ErrClosed) {
			return Value{}, errors.New("retirement permitted a nested call")
		}
		return Int32(42), nil
	}, cleanup: func() {
		cleanups.Add(1)
		if _, err := p.Call(1); !errors.Is(err, ErrClosed) {
			t.Errorf("cleanup reentry: %v", err)
		}
	}}
	go func() {
		_, err := p.Call(1)
		used <- err
	}()
	<-entered
	go func() { closed <- p.Close() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		p.mu.Lock()
		retired := p.backend == nil
		p.mu.Unlock()
		if retired {
			break
		}
		if time.Now().After(deadline) {
			close(release)
			t.Fatal("plan did not begin retirement")
		}
		runtime.Gosched()
	}
	go func() { closed <- p.Close() }()
	if cleanups.Load() != 0 {
		t.Fatal("cleanup ran during invocation")
	}
	close(release)
	if err := <-used; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := <-closed; err != nil {
			t.Fatal(err)
		}
	}
	if cleanups.Load() != 1 {
		t.Fatalf("cleanup count: %d", cleanups.Load())
	}
}
