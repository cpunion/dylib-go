package clang

import (
	"bytes"
	"fmt"

	"github.com/cpunion/dylib-go/abi"
)

// MSVC can preserve explicit/nested alignment beyond a pragma cap; GNU
// compilers cap it. Reconstruct ordinary MSVC fields outside the outer pragma
// using their measured storage alignment. Preserve types and array counts:
// inserting synthetic byte fields could change homogeneous aggregate lowering.
func (t *cgoTypes) writePragmaRecord(output *bytes.Buffer, index int) error {
	r := t.records[index]
	if err := validateMemberAlignments(r, true); err != nil {
		return err
	}
	fmt.Fprintf(output, "typedef struct %s{\n", r.cAttributes())
	for i, field := range r.Description.Fields {
		declaration, err := t.cField(field.Type, fmt.Sprintf("f%d", i))
		if err != nil {
			return fmt.Errorf("clang: %s.%s: %w", r.Name, field.Name, err)
		}
		packing := "packed,"
		element := field.Type
		for element.Type == abi.Array {
			element = *element.Elem
		}
		if element.Type == abi.I8 || element.Type == abi.U8 || element.Type == abi.Bool {
			// Packing a byte field is redundant and triggers GCC warnings.
			packing = ""
		}
		fmt.Fprintf(output, "%s __attribute__((%saligned(%d)));\n", declaration, packing, r.MemberAlignments[i])
	}
	fmt.Fprintf(output, "} %s;\n", t.cRecordName(index))
	return nil
}
