// Package swiftcall demonstrates an explicit native Swift signature.
package swiftcall

import (
	"encoding/binary"
	"fmt"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abibridge"
)

// Add calls a thin Swift `func add(_ a: Int32, _ b: Int32) -> Int32`.
// The caller supplies a loaded ABIBridge facade and the leased Swift symbol.
func Add(runtime *abibridge.Runtime, symbol *dylib.Symbol, a, b int32) (int32, error) {
	call, err := runtime.Prepare(abibridge.Signature{
		Result: abibridge.Type{Kind: abibridge.Int32},
		Args:   []abibridge.Type{{Kind: abibridge.Int32}, {Kind: abibridge.Int32}},
	})
	if err != nil {
		return 0, err
	}
	defer call.Close()
	left, right := make([]byte, 4), make([]byte, 4)
	binary.LittleEndian.PutUint32(left, uint32(a))
	binary.LittleEndian.PutUint32(right, uint32(b))
	result, err := call.Invoke(symbol, left, right)
	if err != nil {
		return 0, err
	}
	if len(result) != 4 {
		return 0, fmt.Errorf("unexpected Swift result size: %d", len(result))
	}
	return int32(binary.LittleEndian.Uint32(result)), nil
}
