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
	members := recordMembers(r)
	starts := bitfieldUnitStarts(r, members)
	paddingStarts := make(map[int]bool)
	for position, member := range members {
		if member.padding >= 0 {
			paddingStarts[member.padding] = starts[position]
		}
	}
	padding := 0
	for j := 0; j <= len(r.Description.Fields); j++ {
		for padding < len(r.Padding) && r.Padding[padding].Before == j {
			p := r.Padding[padding]
			if p.Width != 0 && p.Aligned != 0 && paddingStarts[padding] {
				// MinGW's packing pragma suppresses this request. Reproduce
				// the original bit position measured from a compiler constant.
				fmt.Fprint(&body, "unsigned char :0;\n")
				prefix := fmt.Sprintf("%s_unnamed%d", name, padding)
				fmt.Fprintf(output, "typedef struct __attribute__((packed)) {\n%s unsigned char marker;\n} %s;\n", body.String(), prefix)
				end := fmt.Sprintf("offsetof(%s,marker)", prefix)
				fmt.Fprintf(output, "_Static_assert(%s<=%d && %d-%s<%d,\"incompatible unnamed alignment\");\n", end, p.Offset, p.Offset, end, p.Aligned)
				for b := uint64(0); b < p.Aligned-1; b++ {
					fmt.Fprintf(&body, "unsigned char :(%d-%s>%d?8:0);\n", p.Offset, end, b)
				}
				fmt.Fprint(&body, "unsigned char :0;\n")
			}
			kind := bitfieldBaseType(p.Type)
			if p.Width == 0 {
				kind = "unsigned char"
			}
			attributes := ""
			if p.Width != 0 {
				attributes = p.cAttributes()
			}
			fmt.Fprintf(&body, "%s :%d%s;\n", kind, p.Width, attributes)
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
				alignment := uint64(8)
				if p.Aligned > alignment {
					alignment = p.Aligned
				}
				fmt.Fprintf(output, "_Static_assert(%s<=%d && %d-%s<=%d,\"incompatible zero-width boundary\");\n", end, p.Offset, p.Offset, end, alignment-1)
				for b := uint64(0); b < alignment-1; b++ {
					fmt.Fprintf(&body, "unsigned char :(%d-%s>%d?8:0);\n", p.Offset, end, b)
				}
			}
			padding++
		}
		if j == len(r.Description.Fields) {
			break
		}
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
		declaration, err := t.cNativeField(r, j)
		if err != nil {
			return err
		}
		if bit := r.bitfieldAt(j); bit != nil {
			declaration += fmt.Sprintf(":%d", bit.Width)
		}
		t.writeNativeField(&body, r, j, declaration, r.cFieldAttributes(j))
	}
	fmt.Fprintf(output, "typedef struct __attribute__((packed,aligned(%d))) {\n%s} %s;\n#pragma pack(pop)\n", r.Layout.Alignment, body.String(), name)
	return nil
}
