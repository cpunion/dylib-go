// Package cunion demonstrates selected C union members through compiled facades.
package cunion

import (
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/cunion"
)

// Functions selects active members; the C compiler handles the actual union ABI.
func Functions() []cunion.Function {
	integer := cunion.Value{Type: abi.I32, Union: "union Number", Member: "i"}
	double := cunion.Value{Type: abi.F64, Union: "union Number", Member: "d"}
	return []cunion.Function{
		{Name: "number_add", Symbol: "union_add", Args: []cunion.Value{integer, integer}, Result: integer},
		{Name: "target", Symbol: "union_double", Args: []cunion.Value{double}, Result: double},
	}
}

// Generate renders the selected members as C17 exports.
func Generate(header string) (*cunion.Facade, error) {
	return cunion.Generate(header, Functions())
}
