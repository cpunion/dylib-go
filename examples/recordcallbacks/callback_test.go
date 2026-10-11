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
			value, err := abi.StructValue(record.Description, abi.Int32(0), abi.Ptr(address))
			if err != nil {
				return err
			}
			got, err := dynamic.Call(value, abi.Int32(20), abi.Int32(21))
			if err != nil || int32(got.Bits) != 42 {
				t.Fatal("dynamic callback record", got, err)
			}
			return b.Callbacks_store(BindingsRecord0{Add: unsafe.Pointer(address)})
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
