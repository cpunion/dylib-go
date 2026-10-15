// Package cxxexample demonstrates a generated C facade for a C++ class.
package cxxexample

import (
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/cxx"
)

// Generate selects constructor, member and destruction exports. Compile the
// returned CXXSource with the class's own C++17 toolchain/runtime. Its CHeader
// can use compiler/clang.Parse and CgoSource for Go/llgo without libffi.
func Generate(header string) (*cxx.Facade, error) {
	return cxx.Generate(header, []cxx.Class{{
		Name:   "example::Counter",
		Delete: "counter_delete",
		Constructors: []cxx.Constructor{{
			Symbol: "counter_new", Args: []abi.Type{abi.I32},
		}},
		Methods: []cxx.Method{{
			Name: "add", Symbol: "counter_add",
			Signature: abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32}},
		}},
	}})
}
