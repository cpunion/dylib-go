package clang

import (
	"bytes"
	"fmt"

	"github.com/ramblelabs/dylib-go/abi"
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
	fmt.Fprintf(output, "typedef struct %s{\n", t.nativeAttributes(r))
	for i, field := range r.Description.Fields {
		declaration, err := t.cNativeField(r, i)
		if err != nil {
			return fmt.Errorf("clang: %s.%s: %w", r.Name, field.Name, err)
		}
		t.writeNativeField(output, r, i, declaration, storageFieldAttributes(field.Type, r.MemberAlignments[i]))
	}
	fmt.Fprintf(output, "} %s;\n", t.cRecordName(index))
	return nil
}

func storageFieldAttributes(element abi.TypeDesc, alignment uint64) string {
	packing := "packed,"
	for element.Type == abi.Array {
		element = *element.Elem
	}
	if element.Type == abi.I8 || element.Type == abi.U8 || element.Type == abi.Bool {
		// Packing a byte field is redundant and triggers GCC warnings.
		packing = ""
	}
	return fmt.Sprintf(" __attribute__((%saligned(%d)))", packing, alignment)
}
