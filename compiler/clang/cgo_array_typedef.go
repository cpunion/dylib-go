package clang

import (
	"fmt"

	"github.com/cpunion/dylib-go/abi"
)

// Clang permits an over-aligned array alias inside another array even when its
// size is not a multiple of alignment. GCC rejects that element stride. Keep
// generation portable by rejecting this unqualified form before emitting C.
func (t *cgoTypes) validateArrayTypedefStrides(record Record) error {
	for _, field := range record.FieldTypeAlignments {
		dimensions, leaf := arrayDimensions(record.Description.Fields[field.Position].Type)
		if field.ArrayDepth == 0 || field.ArrayDepth == len(dimensions) {
			continue
		}
		size, err := t.arrayLeafSize(leaf)
		if err != nil {
			return err
		}
		for layer := 0; layer < field.ArrayDepth; layer++ {
			size *= uint64(dimensions[len(dimensions)-1-layer])
		}
		if size%field.Aligned != 0 {
			return fmt.Errorf("clang: %s.%s: array typedef alignment %d does not divide element stride %d", record.Name, record.Description.Fields[field.Position].Name, field.Aligned, size)
		}
	}
	return nil
}

func (t *cgoTypes) arrayLeafSize(d abi.TypeDesc) (uint64, error) {
	switch d.Type {
	case abi.I8, abi.U8, abi.Bool:
		return 1, nil
	case abi.I16, abi.U16:
		return 2, nil
	case abi.I32, abi.U32, abi.F32:
		return 4, nil
	case abi.I64, abi.U64, abi.F64:
		return 8, nil
	case abi.Pointer:
		return uint64(t.pointerSize), nil
	case abi.Struct:
		i, err := t.recordIndex(d)
		if err != nil {
			return 0, err
		}
		return t.records[i].Layout.Size, nil
	}
	return 0, fmt.Errorf("clang: unsupported array typedef element type %d", d.Type)
}
