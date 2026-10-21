package cxxexample

import (
	"github.com/ramblelabs/dylib-go/abi"
	"github.com/ramblelabs/dylib-go/compiler/cxx"
)

// NativePointerClasses selects named pointee types using the existing opaque
// address transport. Aliases can name scalar, void, enum, array, union or pointer
// types; C++ supplies their layout and pointer operations. Borrowed factory
// results require the provider to remain alive and must not be deleted.
func NativePointerClasses() []cxx.Class {
	pointee := func(name string, constant bool) cxx.ObjectType {
		return cxx.ObjectType{Name: "example::" + name, Kind: cxx.ObjectPointer, Const: constant}
	}
	class := cxx.Class{Name: "example::NativePointers"}
	for _, entry := range []struct {
		method, name string
		constant     bool
	}{
		{"numbers", "Number", false}, {"readonly", "Number", true}, {"empty", "Number", false},
		{"slot", "Slot", false}, {"readonly_slot", "Slot", true}, {"rows", "Row", false},
		{"text", "Character", true}, {"mode", "Mode", false}, {"payload", "Payload", false},
	} {
		native := pointee(entry.name, entry.constant)
		class.Methods = append(class.Methods, cxx.Method{
			Name: entry.method, Symbol: "pointer_" + entry.method, Static: true,
			Signature: abi.Signature{Result: abi.Pointer}, ObjectResult: &native,
		})
	}
	for _, entry := range []struct {
		method, name string
		constant     bool
		result       abi.Type
		tail         []abi.Type
	}{
		{"value", "Number", true, abi.I32, nil},
		{"add", "Number", false, abi.I32, []abi.Type{abi.I32}},
		{"advance", "Slot", false, abi.I32, nil},
		{"read_slot", "Slot", true, abi.I32, nil},
		{"row_sum", "Row", true, abi.I32, nil},
		{"parse", "Character", true, abi.I32, nil},
		{"opaque", "Opaque", false, abi.Pointer, nil},
		{"read_mode", "Mode", true, abi.I32, nil},
		{"read_integer", "Payload", true, abi.I32, nil},
		{"set_decimal", "Payload", false, abi.F64, []abi.Type{abi.F64}},
		{"read_decimal", "Payload", true, abi.F64, nil},
	} {
		native := pointee(entry.name, entry.constant)
		method := cxx.Method{
			Name: entry.method, Symbol: "pointer_" + entry.method, Static: true,
			Signature:  abi.Signature{Result: entry.result, Args: append([]abi.Type{abi.Pointer}, entry.tail...)},
			ObjectArgs: []cxx.ObjectArg{{Index: 0, Type: native}},
		}
		if entry.result == abi.Pointer {
			method.ObjectResult = &native
		}
		class.Methods = append(class.Methods, method)
	}
	return []cxx.Class{class}
}
