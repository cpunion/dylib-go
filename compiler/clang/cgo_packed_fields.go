package clang

import (
	"bytes"
	"fmt"
	"math/bits"

	"github.com/cpunion/dylib-go/abi"
)

type packedBitfieldUnit struct {
	first, last int
}

// MinGW ignores member packed attributes on MS-layout bitfields. An anonymous
// record with a scoped packing pragma preserves one producer allocation unit
// without changing the natural alignment of neighboring ordinary members.
func packedBitfieldUnits(r Record) ([]packedBitfieldUnit, error) {
	if r.Packed {
		return nil, nil // Record-wide packing already has its own bridge.
	}
	packed := false
	for _, position := range r.PackedFields {
		packed = packed || r.bitfieldAt(position) != nil
	}
	if !packed {
		return nil, nil
	}
	if len(r.Padding) != 0 {
		return nil, fmt.Errorf("clang: member-packed bitfields with unnamed padding in %s require a separate Windows adapter", r.Name)
	}
	var units []packedBitfieldUnit
	for first := 0; first < len(r.Description.Fields); {
		bit := r.bitfieldAt(first)
		if bit == nil {
			first++
			continue
		}
		width := bitfieldTypeBits(r.Description.Fields[first].Type.Type)
		if r.Description.Fields[first].Type.Type == abi.Bool {
			width = 8 // A Boolean bitfield still owns a byte allocation unit.
		}
		start := r.Layout.Offsets[first] * 8
		end := start + uint64(width)
		last := first
		for next := first + 1; next < len(r.Description.Fields); next++ {
			field := r.bitfieldAt(next)
			if field == nil {
				break
			}
			at := r.Layout.Offsets[next]*8 + uint64(bits.TrailingZeros8(field.Mask[0]))
			if at >= end || at+uint64(field.Width) > end {
				break
			}
			last = next
		}
		if r.fieldPacked(first) {
			units = append(units, packedBitfieldUnit{first, last})
		}
		first = last + 1
	}
	return units, nil
}

func beginPackedBitfieldUnit(output *bytes.Buffer) {
	fmt.Fprint(output, "#if defined(_WIN32) && !defined(_MSC_VER)\n#pragma pack(push,1)\nstruct {\n#endif\n")
}

func endPackedBitfieldUnit(output *bytes.Buffer, alignment uint64) {
	fmt.Fprint(output, "#if defined(_WIN32) && !defined(_MSC_VER)\n}\n#pragma pack(pop)\n")
	if alignment != 0 {
		fmt.Fprintf(output, "__attribute__((aligned(%d)))", alignment)
	}
	fmt.Fprint(output, ";\n#endif\n")
}
