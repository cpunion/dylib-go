//go:build libffi && cgo

package abi

import (
	"sync"
	"testing"
)

func TestPreparedAggregateAndLazyPointeeLayouts(t *testing.T) {
	desc := TypeDesc{Type: Struct, Fields: []Field{{Name: "a", Type: TypeDesc{Type: I32}}, {Name: "b", Type: TypeDesc{Type: I32}}}}
	plan, err := Prepare(Signature{Result: Struct, ResultType: &desc, Args: []Type{Struct, Struct}, ArgTypes: []TypeDesc{desc, desc.Clone()}})
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	b := plan.backend.(*ffiBackend)
	if b.result != b.args[0] || b.result != b.args[1] || len(b.pool.allocations) != 1 {
		t.Fatal("prepared equal aggregates did not share their owned layout")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := b.pointeeLayout(desc)
			if err != nil || n != b.result {
				t.Errorf("concurrent owned layout: %p, %v", n, err)
			}
		}()
	}
	wg.Wait()
	if len(b.pool.allocations) != 1 {
		t.Fatal("cache allocated duplicate native layouts")
	}
}

func TestLazyLayoutFailureRollsBack(t *testing.T) {
	plan, err := Prepare(Signature{})
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	b := plan.backend.(*ffiBackend)
	desc := TypeDesc{Type: Array, Len: 10000, Elem: &TypeDesc{Type: I64}}
	if err := desc.Validate(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := b.pointeeLayout(desc); err == nil {
			t.Fatal("accepted an oversized temporary layout")
		}
		if len(b.pool.allocations) != 0 || len(b.pool.layouts) != 0 {
			t.Fatal("failed lazy construction retained native allocations")
		}
	}
	if _, err := b.pointeeLayout(TypeDesc{Type: Struct, Fields: []Field{{Type: TypeDesc{Type: I32}}}}); err != nil {
		t.Fatal("valid construction after rollback failed:", err)
	}
}
