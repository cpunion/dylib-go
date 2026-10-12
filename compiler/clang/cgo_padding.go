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
				end := fmt.Sprintf("sizeof(%s)", prefix)
				marker := ""
				if len(r.FieldAlignments) != 0 {
					// An aligned member can add tail padding to the prefix. The
					// marker observes the next byte rather than that rounded size.
					marker = "unsigned char marker;\n"
					end = fmt.Sprintf("offsetof(%s,marker)", prefix)
				}
				fmt.Fprintf(output, "typedef struct __attribute__((packed)) {\n%s%s} %s;\n", body.String(), marker, prefix)
				fmt.Fprintf(output, "_Static_assert(%s<=%d && %d-%s<=7,\"incompatible zero-width boundary\");\n", end, p.Offset, p.Offset, end)
				for b := 0; b < 7; b++ {
					fmt.Fprintf(&body, "unsigned char :(%d-%s>%d?8:0);\n", p.Offset, end, b)
				}
			}
			padding++
		}
		if j == len(r.Description.Fields) {
			break
		}
		field := r.Description.Fields[j]
		if alignment := r.fieldAlignment(j); alignment != 0 {
			// MinGW's record packing pragma overrides member aligned(N).
			// End the preceding allocation unit and reproduce the producer's
			// field boundary with unnamed byte fields, preserving the value ABI.
			fmt.Fprint(&body, "unsigned char :0;\n")
			prefix := fmt.Sprintf("%s_aligned%d", name, j)
			fmt.Fprintf(output, "typedef struct __attribute__((packed)) {\n%s unsigned char marker;\n} %s;\n", body.String(), prefix)
			end := fmt.Sprintf("offsetof(%s,marker)", prefix)
			offset := r.Layout.Offsets[j]
			fmt.Fprintf(output, "_Static_assert(%s<=%d && %d-%s<%d,\"incompatible field alignment\");\n", end, offset, offset, end, alignment)
			for b := uint64(0); b < alignment-1; b++ {
				fmt.Fprintf(&body, "unsigned char :(%d-%s>%d?8:0);\n", offset, end, b)
			}
			fmt.Fprint(&body, "unsigned char :0;\n")
		}
		declaration, err := t.cField(field.Type, fmt.Sprintf("f%d", j))
		if err != nil {
			return err
		}
		if bit := r.bitfieldAt(j); bit != nil {
			declaration += fmt.Sprintf(":%d", bit.Width)
		}
		declaration += r.cFieldAlignment(j)
		fmt.Fprintf(&body, "%s;\n", declaration)
	}
	fmt.Fprintf(output, "typedef struct __attribute__((packed,aligned(%d))) {\n%s} %s;\n#pragma pack(pop)\n", r.Layout.Alignment, body.String(), name)
	return nil
}
