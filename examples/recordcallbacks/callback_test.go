//go:build libffi && cgo && (linux || darwin || windows)

package recordcallbacks

import (
	"testing"
	"unsafe"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
)

func TestRetainedGoCallbackField(t *testing.T) {
	withFixtures(t, func(t *testing.T, path string) {
		s, b := open(t, path)
		record, err := _BindingsDeclarations.ForHostRecord("Callbacks")
		if err != nil {
			t.Fatal(err)
		}
		prototype, err := record.LookupFunctionPointer("add")
		if err != nil {
			t.Fatal(err)
		}
		offset := int32(1)
		callback, err := abi.NewCallback(prototype.Signature, func(args []abi.Value) (abi.Value, error) {
			return abi.Int32(int32(args[0].Bits) + int32(args[1].Bits) + offset), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		defer callback.Close()
		owner, err := s.Resolve("callbacks_store")
		if err != nil {
			t.Fatal(err)
		}
		registration, err := dylib.NewRegistration(dylib.RegistrationResources{Symbols: []*dylib.Symbol{owner}, Callbacks: []*abi.Callback{callback}}, func(dylib.RegistrationLeases) error {
			return b.Callbacks_clear()
		})
		if err != nil {
			t.Fatal(err)
		}
		defer registration.Close()
		declaration, err := _BindingsDeclarations.ForHost("callbacks_apply")
		if err != nil {
			t.Fatal(err)
		}
		dynamic, err := s.Bind(declaration.Symbol, declaration.Signature)
		if err != nil {
			t.Fatal(err)
		}
		err = registration.WithLeases(func(leases dylib.RegistrationLeases) error {
			address, err := leases.Callbacks[0].Address()
			if err != nil {
				return err
			}
			pointer, err := leases.Callbacks[0].Pointer()
			if err != nil || pointer == nil || uintptr(pointer) != address {
				t.Fatal("leased pointer", pointer, err)
			}
			value, err := abi.StructValue(record.Description, abi.Int32(0), abi.Ptr(address))
			if err != nil {
				return err
			}
			got, err := dynamic.Call(value, abi.Int32(20), abi.Int32(21))
			if err != nil || int32(got.Bits) != 42 {
				t.Fatal("dynamic callback record", got, err)
			}
			return b.Callbacks_store(BindingsRecord0{Add: pointer})
		})
		if err != nil {
			t.Fatal(err)
		}
		// The native record survives the address-use block. Registration keeps the
		// entry and capture alive until C clears its field before lease release.
		if got, err := b.Callbacks_run(20, 21); err != nil || got != 42 {
			t.Fatal(got, err)
		}
		if err := registration.Close(); err != nil {
			t.Fatal(err)
		}
		if got, err := b.Callbacks_run(20, 21); err != nil || got != -1 {
			t.Fatal("unregister", got, err)
		}
		if err := callback.Err(); err != nil {
			t.Fatal(err)
		}
		if err := callback.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := callback.Acquire(); err == nil {
			t.Fatal("closed callback still leased")
		}
	})
}

func TestRetainedGoCallbackArray(t *testing.T) {
	withFixtures(t, func(t *testing.T, path string) {
		s, b := open(t, path)
		record, err := _BindingsDeclarations.ForHostRecord("ArrayCallbacks")
		if err != nil {
			t.Fatal(err)
		}
		prototype, err := record.LookupFunctionPointer("entries")
		if err != nil {
			t.Fatal(err)
		}
		offset := int32(1)
		add, err := abi.NewCallback(prototype.Signature, func(args []abi.Value) (abi.Value, error) {
			return abi.Int32(int32(args[0].Bits) + int32(args[1].Bits) + offset), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		defer add.Close()
		subtract, err := abi.NewCallback(prototype.Signature, func(args []abi.Value) (abi.Value, error) {
			return abi.Int32(int32(args[0].Bits) - int32(args[1].Bits) + offset), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		defer subtract.Close()
		owner, err := s.Resolve("callback_array_store")
		if err != nil {
			t.Fatal(err)
		}
		registration, err := dylib.NewRegistration(dylib.RegistrationResources{Symbols: []*dylib.Symbol{owner}, Callbacks: []*abi.Callback{add, subtract}}, func(dylib.RegistrationLeases) error { return b.Callback_array_clear() })
		if err != nil {
			t.Fatal(err)
		}
		defer registration.Close()
		declaration, err := _BindingsDeclarations.ForHost("callback_array_apply")
		if err != nil {
			t.Fatal(err)
		}
		dynamic, err := s.Bind(declaration.Symbol, declaration.Signature)
		if err != nil {
			t.Fatal(err)
		}
		err = registration.WithLeases(func(leases dylib.RegistrationLeases) error {
			var entries [2]unsafe.Pointer
			for i := range entries {
				entries[i], err = leases.Callbacks[i].Pointer()
				if err != nil {
					return err
				}
			}
			// Exercise the dynamic ABI's nested array copies as well as generated
			// adapters. Pointer values keep the same callback registration leases.
			rowType := *record.Description.Fields[0].Type.Elem
			row, err := abi.ArrayValue(rowType, abi.Ptr(uintptr(entries[0])), abi.Ptr(uintptr(entries[1])))
			if err != nil {
				return err
			}
			matrix, err := abi.ArrayValue(record.Description.Fields[0].Type, row, row)
			if err != nil {
				return err
			}
			value, err := abi.StructValue(record.Description, matrix)
			if err != nil {
				return err
			}
			got, err := dynamic.Call(value, abi.Int32(1), abi.Int32(1), abi.Int32(63), abi.Int32(22))
			if err != nil || int32(got.Bits) != 42 {
				t.Fatal("dynamic callback array", got, err)
			}
			return b.Callback_array_store(BindingsRecord2{Entries: [2][2]unsafe.Pointer{entries, entries}})
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := b.Callback_array_run(0, 0, 20, 21); err != nil || got != 42 {
			t.Fatal(got, err)
		}
		if got, err := b.Callback_array_run(1, 1, 63, 22); err != nil || got != 42 {
			t.Fatal(got, err)
		}
		if err := registration.Close(); err != nil {
			t.Fatal(err)
		}
		if got, err := b.Callback_array_run(0, 0, 20, 21); err != nil || got != -1 {
			t.Fatal("unregister", got, err)
		}
		if err := add.Err(); err != nil {
			t.Fatal(err)
		}
		if err := subtract.Err(); err != nil {
			t.Fatal(err)
		}
	})
}
