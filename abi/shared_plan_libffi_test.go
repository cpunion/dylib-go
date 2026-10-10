//go:build libffi && cgo

package abi

import (
	"fmt"
	"sync"
	"testing"
)

func TestSharedNativePlanRecordsAndReentry(t *testing.T) {
	array := TypeDesc{Type: Array, Len: 2, Elem: &TypeDesc{Type: I32}}
	desc := TypeDesc{Type: Struct, Fields: []Field{{Name: "depth", Type: TypeDesc{Type: I32}}, {Name: "values", Type: array}}}
	signature := Signature{Result: Struct, ResultType: &desc, Args: []Type{Struct}, ArgTypes: []TypeDesc{desc}}
	first, err := Prepare(signature)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	backend := first.backend.(*ffiBackend)
	renamed := signature.Clone()
	renamed.ResultType.Fields[1].Name = "result"
	renamed.ArgTypes[0].Fields[1].Name = "input"
	second, err := first.Share(renamed)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if first.backend != second.backend || first.backend.(*sharedCallBackend).backend != backend {
		t.Fatal("shared plans allocated independent native CIFs")
	}
	var address uintptr
	callback, err := NewCallback(signature, func(args []Value) (Value, error) {
		value := args[0]
		if value.Aggregate.Fields[0] == Int32(0) {
			return value, nil
		}
		value.Aggregate.Fields[0] = Int32(0)
		return second.Call(address, value)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	if err := callback.WithAddress(func(entry uintptr) error {
		address = entry
		var workers sync.WaitGroup
		for i := 0; i < 8; i++ {
			workers.Add(1)
			go func(seed int32) {
				defer workers.Done()
				values, err := ArrayValue(array, Int32(seed), Int32(42-seed))
				if err != nil {
					t.Error(err)
					return
				}
				for j, plan := range []*CallPlan{first, second} {
					value, err := StructValue(desc, Int32(1), values)
					if err != nil {
						t.Error(err)
						return
					}
					got, err := plan.Call(entry, value)
					name := "values"
					if j == 1 {
						name = "result"
					}
					if err != nil {
						t.Error(err)
						return
					}
					if got.Aggregate.Type.Fields[1].Name != name || got.Aggregate.Fields[0] != Int32(0) || got.Aggregate.Fields[1].Aggregate.Fields[0] != Int32(seed) {
						t.Error("shared native call changed logical metadata or values")
					}
				}
			}(int32(i))
		}
		workers.Wait()
		first.Close()
		value, err := StructValue(desc, Int32(0), Zero(array))
		if err != nil {
			return err
		}
		if _, err := second.Call(entry, value); err != nil {
			return fmt.Errorf("surviving native plan: %w", err)
		}
		return callback.Err()
	}); err != nil {
		t.Fatal(err)
	}
	if len(backend.records) > maxIdleRecordBuffers || backend.recordBytes > maxIdleRecordBytes {
		t.Fatal("sharing multiplied the idle native buffer budget")
	}
	second.Close()
	if len(backend.records) != 0 || len(backend.pool.allocations) != 0 {
		t.Fatal("final shared close retained native resources")
	}
}

func TestSharedNativePlanPointeeContracts(t *testing.T) {
	signature := Signature{Result: Pointer, Args: []Type{Pointer}}
	opaque, err := Prepare(signature)
	if err != nil {
		t.Fatal(err)
	}
	defer opaque.Close()
	scalar := TypeDesc{Type: Pointer, Elem: &TypeDesc{Type: I32}}
	signature.ArgTypes = []TypeDesc{scalar}
	integer, err := opaque.Share(signature)
	if err != nil {
		t.Fatal(err)
	}
	defer integer.Close()
	array := TypeDesc{Type: Array, Len: 2, Elem: &TypeDesc{Type: F64}}
	signature.ArgTypes[0] = TypeDesc{Type: Pointer, Elem: &array}
	floats, err := opaque.Share(signature)
	if err != nil {
		t.Fatal(err)
	}
	defer floats.Close()
	large := TypeDesc{Type: Array, Len: 8193, Elem: &TypeDesc{Type: U64}}
	signature.ArgTypes[0] = TypeDesc{Type: Pointer, Elem: &large}
	oversized, err := opaque.Share(signature)
	if err != nil {
		t.Fatal(err)
	}
	defer oversized.Close()
	callback, err := NewCallback(Signature{Result: Pointer, Args: []Type{Pointer}}, func(args []Value) (Value, error) { return args[0], nil })
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	if err := callback.WithAddress(func(address uintptr) error {
		number := Int32(42)
		values, err := ArrayValue(array, Float64(20), Float64(22))
		if err != nil {
			return err
		}
		if _, err := integer.Call(address, AddressOf(&values)); err == nil {
			return fmt.Errorf("sharing erased a declared scalar pointee contract")
		}
		if _, err := floats.Call(address, AddressOf(&number)); err == nil {
			return fmt.Errorf("sharing erased a declared array pointee contract")
		}
		for _, test := range []struct {
			plan  *CallPlan
			value *Value
		}{{integer, &number}, {floats, &values}, {opaque, &values}} {
			got, err := test.plan.Call(address, AddressOf(test.value))
			if err != nil || got.Pointee != test.value {
				return fmt.Errorf("shared temporary pointer identity: %+v, %v", got, err)
			}
		}
		huge := Zero(large)
		if _, err := oversized.Call(address, AddressOf(&huge)); err == nil {
			return fmt.Errorf("oversized copy bypassed the native storage limit")
		}
		if got, err := oversized.Call(address, Ptr(0x1234)); err != nil || got != Ptr(0x1234) {
			return fmt.Errorf("raw oversized pointer: %+v, %v", got, err)
		}
		if got, err := integer.Call(address, AddressOf(&number)); err != nil || got.Pointee != &number || number != Int32(42) {
			return fmt.Errorf("failed pointee layout poisoned shared resources: %+v, %v", got, err)
		}
		return callback.Err()
	}); err != nil {
		t.Fatal(err)
	}
}
