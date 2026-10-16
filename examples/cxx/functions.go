package cxxexample

import (
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/cxx"
)

// FunctionClasses selects exact native function-pointer arguments and results.
// CallbackBox retains its entry until shutdown/destruction; callers keep both
// callback and provider leases alive for that entire native lifetime.
func FunctionClasses() []cxx.Class {
	add := abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}
	method := func(name string, result abi.Type, args ...abi.Type) cxx.Method {
		return cxx.Method{Name: name, Symbol: "callback_" + name, Signature: abi.Signature{Result: result, Args: args}}
	}
	factory, empty := method("factory", abi.Pointer, abi.Bool), method("empty", abi.Pointer)
	factory.Static, empty.Static = true, true
	factory.FunctionResult, empty.FunctionResult = &add, &add
	invoke := method("invoke", abi.I32, abi.I32, abi.I32)
	invoke.Const = true
	apply := method("apply", abi.I32, abi.Pointer, abi.I32, abi.I32)
	apply.Static, apply.FunctionArgs = true, []cxx.FunctionArg{{Index: 0, Signature: add}}
	exchange := method("exchange", abi.Pointer, abi.Pointer)
	exchange.FunctionArgs, exchange.FunctionResult = []cxx.FunctionArg{{Index: 0, Signature: add}}, &add
	mixed := method("mixed", abi.I32, abi.I32, abi.Pointer, abi.Pointer, abi.Pointer)
	mixed.Const = true
	mixed.ObjectArgs = []cxx.ObjectArg{{Index: 1, Type: cxx.ObjectType{Name: "example::Counter", Kind: cxx.ObjectReference, Const: true}}}
	mixed.FunctionArgs = []cxx.FunctionArg{{Index: 3, Signature: add}, {Index: 2, Signature: add}}
	wide := method("wide", abi.F64, abi.Pointer, abi.I8, abi.F32, abi.U64)
	wide.Static, wide.FunctionArgs = true, []cxx.FunctionArg{{Index: 0, Signature: abi.Signature{Result: abi.F64, Args: []abi.Type{abi.I8, abi.F32, abi.U64}}}}
	notify := method("notify", abi.Void, abi.Pointer, abi.I32, abi.Pointer)
	notify.Static, notify.FunctionArgs = true, []cxx.FunctionArg{{Index: 0, Signature: abi.Signature{Result: abi.Void, Args: []abi.Type{abi.I32, abi.Pointer}}}}
	echo := method("echo", abi.Pointer, abi.Pointer, abi.Pointer)
	echo.Static, echo.FunctionArgs = true, []cxx.FunctionArg{{Index: 0, Signature: abi.Signature{Result: abi.Pointer, Args: []abi.Type{abi.Pointer}}}}
	return []cxx.Class{{
		Name: "example::CallbackBox", Delete: "callback_delete",
		Constructors: []cxx.Constructor{{Symbol: "callback_new", Args: []abi.Type{abi.Pointer}, FunctionArgs: []cxx.FunctionArg{{Index: 0, Signature: add}}}},
		Methods:      []cxx.Method{factory, empty, invoke, apply, exchange, mixed, wide, notify, echo, method("shutdown", abi.I32)},
	}}
}
