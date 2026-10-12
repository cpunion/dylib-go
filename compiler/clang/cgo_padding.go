package clang

import (
	"bytes"
	"fmt"
)

// MinGW zero-width fields can raise packed alignment despite #pragma pack.
// Preserve MS allocation units but use byte-aligned zero-width boundaries,
// followed by the producer's measured gap. Prefix sizeof expressions account
// for the host compiler's current allocation unit without requiring gcc_struct.
func (t *cgoTypes) writePackedPadding(output *bytes.Buffer, index int) error {
	r, name := t.records[index], t.cRecordName(index)
	var body bytes.Buffer
	fmt.Fprint(output, "#pragma pack(push,1)\n")
	padding := 0
	for j := 0; j <= len(r.Description.Fields); j++ {
		for padding < len(r.Padding) && r.Padding[padding].Before == j {
			p := r.Padding[padding]
			kind := bitfieldBaseType(p.Type)
			if p.Width == 0 {
				kind = "unsigned char"
			}
			fmt.Fprintf(&body, "%s :%d;\n", kind, p.Width)
			if p.Width == 0 {
				prefix := fmt.Sprintf("%s_padding%d", name, padding)
				fmt.Fprintf(output, "typedef struct __attribute__((packed)) {\n%s} %s;\n", body.String(), prefix)
				fmt.Fprintf(output, "_Static_assert(sizeof(%s)<=%d && %d-sizeof(%s)<=7,\"incompatible zero-width boundary\");\n", prefix, p.Offset, p.Offset, prefix)
				for b := 0; b < 7; b++ {
					fmt.Fprintf(&body, "unsigned char :(%d-sizeof(%s)>%d?8:0);\n", p.Offset, prefix, b)
				}
			}
			padding++
		}
		if j == len(r.Description.Fields) {
			break
		}
		field := r.Description.Fields[j]
		declaration, err := t.cField(field.Type, fmt.Sprintf("f%d", j))
		if err != nil {
			return err
		}
		if bit := r.bitfieldAt(j); bit != nil {
			declaration += fmt.Sprintf(":%d", bit.Width)
		}
		fmt.Fprintf(&body, "%s;\n", declaration)
	}
	fmt.Fprintf(output, "typedef struct __attribute__((packed,aligned(%d))) {\n%s} %s;\n#pragma pack(pop)\n", r.Layout.Alignment, body.String(), name)
	return nil
}
