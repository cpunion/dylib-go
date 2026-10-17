// Package cunion demonstrates C union members and byte views through compiled facades.
package cunion

import (
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/cunion"
)

// Functions selects member/byte views; the C compiler handles the actual union ABI.
func Functions() []cunion.Function {
	integer := cunion.Value{Type: abi.I32, Union: "union Number", Member: "i"}
	pair := cunion.Value{Type: abi.Struct, Record: "struct Pair", Union: "union Records", Member: "pair"}
	double := cunion.Value{Type: abi.F64, Union: "union Number", Member: "d"}
	nested := cunion.Value{Type: abi.I32, Union: "union Nested", Member: "payload.inner.pair.a"}
	array := cunion.Value{Type: abi.Array, Array: "IntPair", Union: "union Arrays", Member: "ints"}
	bytes := cunion.Value{Type: abi.Array, Union: "union RawSmall", Bytes: 8}
	return []cunion.Function{
		{Name: "number_add", Symbol: "union_add", Args: []cunion.Value{integer, integer}, Result: integer},
		{Name: "target", Symbol: "union_double", Args: []cunion.Value{double}, Result: double},
		{Name: "record_pair_add", Symbol: "union_pair", Args: []cunion.Value{pair, {Type: abi.Struct, Record: "struct Pair"}}, Result: pair},
		{Name: "nested_add", Symbol: "union_nested_add", Args: []cunion.Value{nested, {Type: abi.I32}}, Result: nested},
		{Name: "arrays_add", Symbol: "union_arrays_add", Args: []cunion.Value{array, {Type: abi.I32}}, Result: array},
		{Name: "raw_small_echo", Symbol: "union_bytes_small", Args: []cunion.Value{bytes}, Result: bytes},
	}
}

// Generate renders the selected members as C17 exports.
func Generate(header string) (*cunion.Facade, error) {
	return cunion.Generate(header, Functions())
}
