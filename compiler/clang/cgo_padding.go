package clang

import (
	"bytes"
	"fmt"

	"github.com/cpunion/dylib-go/abi"
)

// MinGW zero-width bitfields can raise a packed record's alignment despite
// #pragma pack. On Windows, restore its measured storage with GCC-style packed
// fields and unnamed positive-width gaps. Named field types/prototypes remain
// intact; no synthetic value members change homogeneous-aggregate decisions.
func (t *cgoTypes) writePackedPadding(output *bytes.Buffer, index int) error {
	r := t.records[index]
	fmt.Fprintf(output, "typedef struct __attribute__((packed,gcc_struct,aligned(%d))) {\n", r.Layout.Alignment)
	cursor := uint64(0)
	pad := func(end uint64) error {
		if end < cursor {
			return fmt.Errorf("clang: overlapping native fields in %s", r.Name)
		}
		for end > cursor {
			width := end - cursor
			if width > 64 {
				width = 64
			}
			fmt.Fprintf(output, "uint64_t :%d;\n", width)
			cursor += width
		}
		return nil
	}
	for i, field := range r.Description.Fields {
		start := r.Layout.Offsets[i] * 8
		width, err := t.nativeFieldBits(field.Type)
		if err != nil {
			return err
		}
		bit := r.bitfieldAt(i)
		if bit != nil {
			for mask := bit.Mask[0]; mask&1 == 0; mask >>= 1 {
				start++
			}
			width = uint64(bit.Width)
		}
		if err := pad(start); err != nil {
			return err
		}
		declaration, err := t.cField(field.Type, fmt.Sprintf("f%d", i))
		if err != nil {
			return err
		}
		if bit != nil {
			declaration += fmt.Sprintf(":%d", bit.Width)
		}
		fmt.Fprintf(output, "%s;\n", declaration)
		cursor += width
	}
	if err := pad(r.Layout.Size * 8); err != nil {
		return err
	}
	fmt.Fprintf(output, "} %s;\n", t.cRecordName(index))
	return nil
}

func (t *cgoTypes) nativeFieldBits(d abi.TypeDesc) (uint64, error) {
	switch d.Type {
	case abi.Array:
		size, err := t.nativeFieldBits(*d.Elem)
		if err != nil || size > 65536*8/uint64(d.Len) {
			return 0, fmt.Errorf("clang: invalid native array storage")
		}
		return size * uint64(d.Len), nil
	case abi.Struct:
		index, err := t.recordIndex(d)
		if err != nil {
			return 0, err
		}
		return t.records[index].Layout.Size * 8, nil
	case abi.Pointer:
		return t.pointerSize * 8, nil
	case abi.Bool, abi.I8, abi.U8:
		return 8, nil
	case abi.I16, abi.U16:
		return 16, nil
	case abi.I32, abi.U32, abi.F32:
		return 32, nil
	case abi.I64, abi.U64, abi.F64:
		return 64, nil
	}
	return 0, fmt.Errorf("clang: unsupported native field storage")
}
