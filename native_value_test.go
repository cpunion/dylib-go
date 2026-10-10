package dylib

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestOwnedNativeStorageWithC(t *testing.T) {
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			session := nativeABILibrary(t, "testdata/native_values.c", input)
			bind := func(name string, result abi.Type, arguments ...abi.Type) *Function {
				t.Helper()
				function, err := session.Bind(name, abi.Signature{Result: result, Args: arguments})
				if err != nil {
					t.Fatal(err)
				}
				return function
			}
			invoke := func(function *Function, args ...abi.Value) abi.Value {
				t.Helper()
				value, err := function.Call(args...)
				if err != nil {
					t.Fatal(err)
				}
				return value
			}
			array := abi.TypeDesc{Type: abi.Array, Len: 2, Elem: &abi.TypeDesc{Type: abi.I32}}
			desc := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{
				{Name: "tag", Type: abi.TypeDesc{Type: abi.I8}},
				{Name: "value", Type: abi.TypeDesc{Type: abi.F64}},
				{Name: "truth", Type: abi.TypeDesc{Type: abi.Bool}},
				{Name: "values", Type: array},
				{Name: "next", Type: abi.TypeDesc{Type: abi.Pointer}},
			}}
			values, _ := abi.ArrayValue(array, abi.Int32(10), abi.Int32(12))
			initial, _ := abi.StructValue(desc, abi.Int8(-128), abi.Float64(20.5), abi.Boolean(true), values, abi.Ptr(0))
			owner, err := abi.NewNativeValue(desc, initial)
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Close()
			layout, err := owner.Layout()
			if err != nil {
				t.Fatal(err)
			}
			if got := invoke(bind("owned_size", abi.U64)); got.Bits != layout.Size {
				t.Fatal("C storage size:", got, layout)
			}
			if got := invoke(bind("owned_alignment", abi.U64)); got.Bits != layout.Alignment {
				t.Fatal("C storage alignment:", got, layout)
			}
			offsets := bind("owned_offset", abi.U64, abi.I32)
			for i, offset := range layout.Offsets {
				if got := invoke(offsets, abi.Int32(int32(i))); got.Bits != offset {
					t.Fatal("C storage offset:", i, got, layout)
				}
			}
			lease, err := owner.Acquire()
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			address, err := lease.Address()
			if err != nil {
				t.Fatal(err)
			}
			initial.Aggregate.Fields[4] = abi.Ptr(address)
			if err := lease.Write(initial); err != nil {
				t.Fatal(err)
			}
			store := bind("owned_store", abi.Void, abi.Pointer)
			invoke(store, abi.Ptr(address))
			defer invoke(store, abi.Ptr(0)) // Remove C's registration before lease release.
			runtime.GC()
			if got := invoke(bind("owned_pointer", abi.Pointer)); got != abi.Ptr(address) {
				t.Fatal("C retained address:", got)
			}
			invoke(bind("owned_mutate", abi.Void, abi.I32), abi.Int32(10))
			wantedArray, _ := abi.ArrayValue(array, abi.Int32(20), abi.Int32(22))
			wanted, _ := abi.StructValue(desc, abi.Int8(-127), abi.Float64(22), abi.Boolean(false), wantedArray, abi.Ptr(address))
			got, err := lease.Read()
			if err != nil || !reflect.DeepEqual(got, wanted) {
				t.Fatal("C mutation through retained storage:", got, err, wanted)
			}
			if got := invoke(bind("owned_sum", abi.I32)); got != abi.Int32(42) {
				t.Fatal("C retained value:", got)
			}
			callback, err := abi.NewCallback(abi.Signature{Result: abi.I32, Args: []abi.Type{abi.Pointer}}, func(args []abi.Value) (abi.Value, error) {
				if args[0] != abi.Ptr(address) {
					t.Error("callback native address:", args[0])
				}
				snapshot, err := lease.Read()
				if err != nil {
					return abi.Value{}, err
				}
				items := snapshot.Aggregate.Fields[3].Aggregate.Fields
				return abi.Int32(int32(items[0].Bits) + int32(items[1].Bits)), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer callback.Close()
			if got := callbackCall(t, session, "func owned_callback(unsafe.Pointer)int32", callback); got != abi.Int32(42) || callback.Err() != nil {
				t.Fatal("callback reads owned storage:", got, callback.Err())
			}
			arrayOwner, err := abi.NewNativeValue(array, wantedArray)
			if err != nil {
				t.Fatal(err)
			}
			defer arrayOwner.Close()
			err = arrayOwner.WithAddress(func(address uintptr) error {
				store := bind("array_store", abi.Void, abi.Pointer)
				invoke(store, abi.Ptr(address))
				defer invoke(store, abi.Ptr(0))
				invoke(bind("array_mutate", abi.Void, abi.I32), abi.Int32(1))
				if got := invoke(bind("array_sum", abi.I32)); got != abi.Int32(44) {
					t.Fatal("C array storage:", got)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			arrayRead, err := arrayOwner.Read()
			want, _ := abi.ArrayValue(array, abi.Int32(21), abi.Int32(23))
			if err != nil || !reflect.DeepEqual(arrayRead, want) {
				t.Fatal("C array mutation:", arrayRead, err)
			}
		})
	}
}
