package cxxexample

import (
	"github.com/ramblelabs/dylib-go/abi"
	"github.com/ramblelabs/dylib-go/compiler/cxx"
)

// ObjectClasses selects native copies, borrowed references and typed pointers.
// The executable fixture supplies these classes; C++ owns all object layout.
func ObjectClasses() []cxx.Class {
	object := func(kind cxx.ObjectKind, constant bool) cxx.ObjectType {
		return cxx.ObjectType{Name: "example::Counter", Kind: kind, Const: constant}
	}
	arg := func(kind cxx.ObjectKind, constant bool) []cxx.ObjectArg {
		return []cxx.ObjectArg{{Index: 0, Type: object(kind, constant)}}
	}
	method := func(name string, result abi.Type, args ...abi.Type) cxx.Method {
		return cxx.Method{Name: name, Symbol: "object_" + name, Signature: abi.Signature{Result: result, Args: args}}
	}
	sum, copied := method("sum", abi.I32, abi.Pointer), method("copied", abi.I32, abi.Pointer)
	sum.Const, copied.Const = true, true
	sum.ObjectArgs, copied.ObjectArgs = arg(cxx.ObjectReference, true), arg(cxx.ObjectValue, false)
	mutate := method("mutate", abi.I32, abi.Pointer, abi.I32)
	mutate.ObjectArgs = arg(cxx.ObjectReference, false)
	clone, self, constant := method("clone", abi.Pointer), method("self", abi.Pointer), method("const_self", abi.Pointer)
	clone.Const, constant.Const = true, true
	valueType, referenceType, constantType := object(cxx.ObjectValue, false), object(cxx.ObjectReference, false), object(cxx.ObjectReference, true)
	clone.ObjectResult, self.ObjectResult, constant.ObjectResult = &valueType, &referenceType, &constantType
	pointer, nullable := method("pointer", abi.Pointer, abi.Pointer), method("nullable", abi.I32, abi.Pointer)
	pointer.Static, nullable.Static = true, true
	pointer.ObjectArgs, nullable.ObjectArgs = arg(cxx.ObjectPointer, false), arg(cxx.ObjectPointer, true)
	pointerType := object(cxx.ObjectPointer, false)
	pointer.ObjectResult = &pointerType
	constPointer := method("const_pointer", abi.Pointer, abi.Pointer)
	constPointer.Static, constPointer.ObjectArgs = true, arg(cxx.ObjectPointer, true)
	constPointerType := object(cxx.ObjectPointer, true)
	constPointer.ObjectResult = &constPointerType
	alias := constPointer
	alias.Symbol = "object_const_alias"
	alias.ObjectResult = &cxx.ObjectType{Name: "example::ReadOnlyCounter", Kind: cxx.ObjectPointer}
	mixed := method("mixed", abi.I32, abi.I32, abi.Pointer, abi.Pointer)
	mixed.Const = true
	mixed.ObjectArgs = []cxx.ObjectArg{{Index: 2, Type: pointerType}, {Index: 1, Type: referenceType}}
	mixed.ObjectArgs[0].Type.Const, mixed.ObjectArgs[1].Type.Const = true, true
	live := method("live", abi.I32)
	live.Static = true
	return []cxx.Class{{
		Name: "example::Counter", Delete: "object_delete",
		Constructors: []cxx.Constructor{
			{Symbol: "object_new", Args: []abi.Type{abi.I32}},
			{Symbol: "object_copy", Args: []abi.Type{abi.Pointer}, ObjectArgs: arg(cxx.ObjectReference, true)},
			{Symbol: "object_value_copy", Args: []abi.Type{abi.Pointer}, ObjectArgs: arg(cxx.ObjectValue, false)},
		},
		Methods: []cxx.Method{sum, copied, mutate, clone, self, constant, pointer, nullable, live, constPointer, mixed, alias},
	}, {
		Name: "example::Token", Delete: "token_delete",
		Methods: []cxx.Method{
			{Name: "make", Symbol: "token_make", Static: true, Signature: abi.Signature{Result: abi.Pointer, Args: []abi.Type{abi.I32}}, ObjectResult: &cxx.ObjectType{Name: "example::Token", Kind: cxx.ObjectValue}},
			{Name: "read", Symbol: "token_read", Const: true, Signature: abi.Signature{Result: abi.I32}},
		},
	}, {
		Name:    "example::StaticOnly",
		Methods: []cxx.Method{{Name: "answer", Symbol: "static_answer", Static: true, Signature: abi.Signature{Result: abi.I32}}},
	}, {
		Name: "example::ReadOnlyCounter", Delete: "readonly_delete",
		Constructors: []cxx.Constructor{{Symbol: "readonly_new", Args: []abi.Type{abi.I32}}},
	}}
}
