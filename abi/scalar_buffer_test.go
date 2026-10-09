//go:build libffi && cgo

package abi

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"unsafe"
)

func scalarBufferPlan(t *testing.T, n int) (*CallPlan, *ffiBackend) {
	t.Helper()
	sig := Signature{Result: U64, Args: make([]Type, n)}
	for i := range sig.Args {
		sig.Args[i] = U64
	}
	plan, err := Prepare(sig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { plan.Close() })
	return plan, plan.backend.(*ffiBackend)
}

func TestScalarBufferReuseZeroingAndIsolation(t *testing.T) {
	for _, n := range []int{0, 1, 32, 33, 64} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			_, backend := scalarBufferPlan(t, n)
			first, err := backend.acquireScalars()
			if err != nil {
				t.Fatal(err)
			}
			second, err := backend.acquireScalars()
			if err != nil {
				backend.releaseScalars(first)
				t.Fatal(err)
			}
			defer backend.releaseScalars(second)
			if first == second || first.call == second.call {
				t.Fatal("active calls shared native storage")
			}
			address := first.call
			if n != 0 {
				alignment := unsafe.Alignof(first.bits[0])
				if uintptr(unsafe.Pointer(first.call.bits))%alignment != 0 || uintptr(unsafe.Pointer(first.call.storage))%alignment != 0 || first.call.bits == second.call.bits {
					t.Fatal("scalar storage is unaligned or shared")
				}
			}
			for i := range first.payload {
				first.payload[i] = 0xa5
			}
			result := unsafe.Slice((*byte)(unsafe.Pointer(&first.call.ret)), int(unsafe.Sizeof(first.call.ret)))
			for i := range result {
				result[i] = 0xa5
			}
			first.call.status = -1
			backend.releaseScalars(first)
			for _, memory := range [][]byte{first.payload, result} {
				for _, value := range memory {
					if value != 0 {
						t.Fatal("idle context retained foreign values or addresses")
					}
				}
			}
			if first.call.status != 0 {
				t.Fatal("idle context retained invocation status")
			}
			reused, err := backend.acquireScalars()
			if err != nil {
				t.Fatal(err)
			}
			defer backend.releaseScalars(reused)
			if reused != first || reused.call != address {
				t.Fatal("scalar buffers were not reused")
			}
		})
	}
}

func TestScalarBufferCountLimitAndClose(t *testing.T) {
	plan, backend := scalarBufferPlan(t, 64)
	var works []*scalarBuffer
	for i := 0; i < maxIdleScalarBuffers+2; i++ {
		work, err := backend.acquireScalars()
		if err != nil {
			for _, active := range works {
				backend.releaseScalars(active)
			}
			t.Fatal(err)
		}
		works = append(works, work)
	}
	for _, work := range works {
		backend.releaseScalars(work)
	}
	if len(backend.scalars) != maxIdleScalarBuffers || backend.scalarBytes != uint64(maxIdleScalarBuffers)*works[0].size {
		t.Fatal("idle scalar count/byte accounting is incorrect")
	}
	for _, work := range works[maxIdleScalarBuffers:] {
		if work.call != nil || work.bits != nil || work.payload != nil {
			t.Fatal("evicted scalar storage was retained")
		}
	}
	if err := plan.Close(); err != nil {
		t.Fatal(err)
	}
	if len(backend.scalars) != 0 || backend.scalarBytes != 0 {
		t.Fatal("Close retained idle scalar buffers")
	}
	for _, work := range works {
		if work.call != nil || work.bits != nil || work.payload != nil {
			t.Fatal("Close did not release native scalar storage")
		}
	}
}

func TestScalarBufferByteLimit(t *testing.T) {
	for _, n := range []int{30000, 60000} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			_, backend := scalarBufferPlan(t, n)
			first, err := backend.acquireScalars()
			if err != nil {
				t.Fatal(err)
			}
			second, err := backend.acquireScalars()
			if err != nil {
				backend.releaseScalars(first)
				t.Fatal(err)
			}
			// At 30000 arguments only one context fits; at 60000 neither fits.
			backend.releaseScalars(first)
			backend.releaseScalars(second)
			want := 0
			if n == 30000 {
				want = 1
			}
			if len(backend.scalars) != want || backend.scalarBytes > maxIdleScalarBytes || second.call != nil {
				t.Fatal("idle scalar byte budget was not enforced")
			}
			if want == 0 && (first.call != nil || backend.scalarBytes != 0) || want == 1 && backend.scalarBytes != first.size {
				t.Fatal("oversized context retention/accounting is incorrect")
			}
		})
	}
}

func TestScalarBufferInvocationErrorRecovery(t *testing.T) {
	plan, backend := scalarBufferPlan(t, 1)
	work, err := backend.acquireScalars()
	if err != nil {
		t.Fatal(err)
	}
	backend.releaseScalars(work)
	// Reject before ffi_call; the deliberately invalid address is never used.
	work.call.n = 0
	if _, err := plan.Call(1, Uint64(42)); err == nil || !strings.Contains(err.Error(), "ffi scalar invocation failed") {
		t.Fatalf("native scalar count mismatch: %v", err)
	}
	for _, value := range work.payload {
		if value != 0 {
			t.Fatal("failed invocation retained scalar input")
		}
	}
	work.call.n = 1
	callback, err := NewCallback(Signature{Result: U64, Args: []Type{U64}}, func(args []Value) (Value, error) { return args[0], nil })
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	if err := callback.WithAddress(func(address uintptr) error {
		got, err := plan.Call(address, Uint64(42))
		if err == nil && got != Uint64(42) {
			err = fmt.Errorf("valid scalar call after failure: %+v", got)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestScalarBufferLargeConcurrentAndNestedCalls(t *testing.T) {
	plan, _ := scalarBufferPlan(t, 64)
	// Give every level/caller different arguments. The handler reenters the same
	// prepared plan while the outer native call still owns its result buffer.
	var address uintptr
	callback, err := NewCallback(plan.signature, func(args []Value) (Value, error) {
		depth := args[0].Bits
		var sum uint64
		for i, arg := range args {
			if i != 0 {
				sum += arg.Bits
			}
		}
		if depth == 0 {
			return Uint64(sum), nil
		}
		args[0] = Uint64(depth - 1)
		nested, err := plan.Call(address, args...)
		return Uint64(sum + nested.Bits), err
	})
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	if err := callback.WithAddress(func(entry uintptr) error {
		address = entry
		var workers sync.WaitGroup
		for caller := uint64(1); caller <= 8; caller++ {
			workers.Add(1)
			go func(seed uint64) {
				defer workers.Done()
				args := make([]Value, 64)
				args[0] = Uint64(2)
				var sum uint64
				for i := 1; i < len(args); i++ {
					args[i] = Uint64(seed + uint64(i))
					sum += args[i].Bits
				}
				for i := 0; i < 10; i++ {
					got, err := plan.Call(entry, args...)
					if err != nil || got != Uint64(3*sum) {
						t.Errorf("overlapping/nested scalar result: %+v, %v", got, err)
						return
					}
				}
			}(caller)
		}
		workers.Wait()
		return callback.Err()
	}); err != nil {
		t.Fatal(err)
	}
}
