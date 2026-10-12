package clang

import (
	"fmt"
	"strings"
)

// Naming one nonzero padding field lets a constant reveal its bit position.
// It can increase tail alignment on POSIX, but does not move the field itself.
// Read only the original record's bytes; native layout still comes from the
// original declaration, never this synthetic type.
func (r Record) alignedPaddingProbe(record, padding int) string {
	var source strings.Builder
	if r.MaxFieldAlignment != 0 {
		fmt.Fprintf(&source, "#pragma pack(push,%d)\n", r.MaxFieldAlignment)
	}
	fmt.Fprintf(&source, "const struct %s{\n", r.cAttributes())
	for _, member := range recordMembers(r) {
		if member.padding >= 0 {
			p := r.Padding[member.padding]
			name := ""
			if member.padding == padding {
				name = "value"
			}
			fmt.Fprintf(&source, "%s %s:%d%s;\n", bitfieldBaseType(p.Type), name, p.Width, p.cAttributes())
		} else {
			j := member.field
			if bit := r.bitfieldAt(j); bit != nil {
				fmt.Fprintf(&source, "%s f%d:%d%s;\n", bitfieldBaseType(r.Description.Fields[j].Type.Type), j, bit.Width, r.cFieldAttributes(j))
			} else {
				fmt.Fprintf(&source, "__typeof__(((%s *)0)->%s) f%d%s;\n", r.Name, r.Description.Fields[j].Name, j, r.cFieldAttributes(j))
			}
		}
	}
	p := r.Padding[padding]
	fmt.Fprintf(&source, "} dylib_bitfield_padding_%d_%d = {.value=%s};\n", record, padding, bitfieldValue(p.Type, p.Width))
	if r.MaxFieldAlignment != 0 {
		fmt.Fprint(&source, "#pragma pack(pop)\n")
	}
	return source.String()
}
