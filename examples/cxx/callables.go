package cxxexample

import (
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/cxx"
)

// CallableClasses selects named aliases for std::function and captured lambdas.
// C++ owns their state and layout; operator() uses an exact member signature.
// A std::function retaining a Go callback also needs explicit callback and
// provider leases until its last native copy is destroyed.
func CallableClasses() []cxx.Class {
	add := abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}
	classes := []cxx.Class{}
	for _, entry := range []struct {
		name, prefix string
		constant     bool
	}{
		{"example::Function", "function", true},
		{"example::CapturedAdder", "captured", false},
		{"example::MoveOnlyAdder", "move_only", false},
	} {
		classes = append(classes, cxx.Class{
			Name: entry.name, Delete: entry.prefix + "_delete",
			Methods: []cxx.Method{{Name: "operator()", Symbol: entry.prefix + "_call", Const: entry.constant, Signature: add}},
		})
	}
	classes[0].Constructors = []cxx.Constructor{
		{Symbol: "function_new", Args: []abi.Type{abi.Pointer}, FunctionArgs: []cxx.FunctionArg{{Index: 0, Signature: add}}},
		{Symbol: "function_empty"},
	}
	for i, prefix := range []string{"function", "captured"} {
		classes[i].Constructors = append(classes[i].Constructors, cxx.Constructor{
			Symbol: prefix + "_copy", Args: []abi.Type{abi.Pointer},
			ObjectArgs: []cxx.ObjectArg{{Index: 0, Type: cxx.ObjectType{Name: classes[i].Name, Kind: cxx.ObjectReference, Const: true}}},
		})
	}
	factory := cxx.Class{Name: "example::CallableFactory"}
	for i, name := range []string{"function", "captured", "move_only"} {
		factory.Methods = append(factory.Methods, cxx.Method{
			Name: name, Symbol: name + "_make", Static: true,
			Signature:    abi.Signature{Result: abi.Pointer, Args: []abi.Type{abi.I32}},
			ObjectResult: &cxx.ObjectType{Name: classes[i].Name, Kind: cxx.ObjectValue},
		})
	}
	factory.Methods = append(factory.Methods,
		cxx.Method{Name: "entry", Symbol: "callable_entry", Static: true, Signature: abi.Signature{Result: abi.Pointer}, FunctionResult: &add},
		cxx.Method{Name: "live", Symbol: "callable_live", Static: true, Signature: abi.Signature{Result: abi.I32}},
	)
	return append(classes, factory)
}
