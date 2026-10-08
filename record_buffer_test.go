package dylib

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestNativeRecordCallbackReentry(t *testing.T) {
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			session := nativeABILibrary(t, "testdata/record_buffers.c", input)
			desc := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "a", Type: abi.TypeDesc{Type: abi.I32}}, {Name: "b", Type: abi.TypeDesc{Type: abi.I32}}}}
			fn, err := session.Bind("reenter_record", callbackSignature(t, "func reenter_record(unsafe.Pointer,struct{a,b int32},*struct{a,b int32},int32)int32"))
			if err != nil {
				t.Fatal(err)
			}
			var address uintptr
			var invoke func(int32) (abi.Value, error)
			callback, err := abi.NewCallback(callbackSignature(t, "func callback(int32)int32"), func(args []abi.Value) (abi.Value, error) {
				depth := int32(args[0].Bits)
				if depth < 2 {
					return invoke(depth + 1)
				}
				return abi.Int32(0), nil
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
			address, err = lease.Address()
			if err != nil {
				t.Fatal(err)
			}
			invoke = func(depth int32) (abi.Value, error) {
				value, err := abi.StructValue(desc, abi.Int32(20), abi.Int32(22))
				if err != nil {
					return abi.Value{}, err
				}
				pointee, err := abi.StructValue(desc, abi.Int32(20), abi.Int32(22))
				if err != nil {
					return abi.Value{}, err
				}
				result, err := fn.Call(abi.Ptr(address), value, abi.AddressOf(&pointee), abi.Int32(depth))
				if err == nil && (value.Aggregate.Fields[0] != abi.Int32(20) || pointee.Aggregate.Fields[0] != abi.Int32(20+depth) || pointee.Aggregate.Fields[1] != abi.Int32(22)) {
					err = fmt.Errorf("nested call changed another invocation's values: depth=%d", depth)
				}
				return result, err
			}
			// Exercise new contexts, reuse after nested calls and stable cleanup.
			for i := 0; i < 5; i++ {
				if got, err := invoke(0); err != nil || got != abi.Int32(255) || callback.Err() != nil {
					t.Fatalf("nested record call: %+v, %v, callback=%v", got, err, callback.Err())
				}
			}
		})
	}
}

func TestNativeRecordOpaqueTemporaryShapes(t *testing.T) {
	session := nativeABILibrary(t, "testdata/many_arguments.c", "object")
	fn, err := session.Bind("identity_pointer", abi.Signature{Result: abi.Pointer, Args: []abi.Type{abi.Pointer}})
	if err != nil {
		t.Fatal(err)
	}
	desc := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "a", Type: abi.TypeDesc{Type: abi.I32}}, {Name: "b", Type: abi.TypeDesc{Type: abi.I32}}}}
	for i := 0; i < 10; i++ {
		value := abi.Int32(42)
		if i%2 != 0 {
			value, err = abi.StructValue(desc, abi.Int32(20), abi.Int32(22))
			if err != nil {
				t.Fatal(err)
			}
		}
		got, err := fn.Call(abi.AddressOf(&value))
		if err != nil || got.Bits != 0 || got.Pointee != &value {
			t.Fatalf("opaque temporary identity: %+v, %v", got, err)
		}
		if i%2 == 0 && value != abi.Int32(42) || i%2 != 0 && (value.Aggregate.Fields[0] != abi.Int32(20) || value.Aggregate.Fields[1] != abi.Int32(22)) {
			t.Fatal("a reused context kept the previous temporary shape")
		}
	}
}

func TestNativeRecordMarshalingFailureRecovery(t *testing.T) {
	session := nativeABILibrary(t, "testdata/many_arguments.c", "object")
	fn, err := session.Bind("identity_pointer", abi.Signature{Result: abi.Pointer, Args: []abi.Type{abi.Pointer}})
	if err != nil {
		t.Fatal(err)
	}
	large := abi.Zero(abi.TypeDesc{Type: abi.Array, Len: 10000, Elem: &abi.TypeDesc{Type: abi.U64}})
	for i := 0; i < 3; i++ {
		// The descriptor is valid, but its native temporary exceeds 64 KiB.
		// Failure occurs after acquiring a context, before invoking native code.
		if _, err := fn.Call(abi.AddressOf(&large)); err == nil || !strings.Contains(err.Error(), "64 KiB") {
			t.Fatalf("oversized temporary: %v", err)
		}
		value := abi.Int32(42)
		if got, err := fn.Call(abi.AddressOf(&value)); err != nil || got.Pointee != &value || value != abi.Int32(42) {
			t.Fatalf("valid call after marshaling failure: %+v, %v", got, err)
		}
	}
}

func TestNativeRecordArgumentVectorReuse(t *testing.T) {
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			session := nativeABILibrary(t, "testdata/record_buffers.c", input)
			sig := callbackSignature(t, "func bump_big(struct{a,b,c int64},int64)struct{a,b,c int64}")
			fn, err := session.Bind("bump_big", sig)
			if err != nil {
				t.Fatal(err)
			}
			for i := int64(0); i < 10; i++ {
				value, err := abi.StructValue(sig.ArgumentType(0), abi.Int64(20+i), abi.Int64(21-i), abi.Int64(1))
				if err != nil {
					t.Fatal(err)
				}
				// libffi can rewrite the address vector for structures larger
				// than the register ABI. A reused vector must start from roots.
				got, err := fn.Call(value, abi.Int64(2*i))
				if err != nil || got.Aggregate == nil || got.Aggregate.Fields[0] != abi.Int64(20+3*i) || got.Aggregate.Fields[1] != abi.Int64(21-i) || got.Aggregate.Fields[2] != abi.Int64(1) || value.Aggregate.Fields[0] != abi.Int64(20+i) {
					t.Fatalf("large struct call %d: %+v, %v", i, got, err)
				}
			}
		})
	}
}
