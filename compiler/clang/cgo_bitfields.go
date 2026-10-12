package clang

import (
	"bytes"
	"fmt"
)

func (t *cgoTypes) writeBitfieldMasks(output *bytes.Buffer, index int) {
	r, name := t.records[index], t.cRecordName(index)
	// Volatile bitfield writes preserve bits outside the field's storage mask,
	// including padding next to signed fields, under optimization.
	fmt.Fprintf(output, "static uint64_t %s_mask(int32_t field,uint64_t byte) {\nvolatile %s value;\nvolatile unsigned char *data = (volatile unsigned char *)&value;\nfor (uint64_t i=0;i<sizeof(value);++i) data[i]=0;\nswitch(field) {\n", name, name)
	for _, f := range r.Bitfields {
		fmt.Fprintf(output, "case %d:value.f%d=%s;break;\n", f.Position, f.Position, bitfieldValue(r.Description.Fields[f.Position].Type.Type, f.Width))
	}
	fmt.Fprint(output, "default:return UINT64_MAX;\n}\nif (byte==UINT64_MAX) {\nfor (uint64_t i=0;i<sizeof(value);++i) if (data[i]) return i;\nreturn UINT64_MAX;\n}\nreturn byte<sizeof(value)?data[byte]:UINT64_MAX;\n}\n")
}
