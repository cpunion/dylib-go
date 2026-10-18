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
	aliases, kinds, paddingKinds := r.bitfieldTypedefProbes(fmt.Sprintf("dylib_padding_%d_%d_bittypedef", record, padding))
	source.WriteString(aliases)
	if r.MaxFieldAlignment != 0 {
		fmt.Fprintf(&source, "#pragma pack(push,%d)\n", r.MaxFieldAlignment)
	}
	var body strings.Builder
	for _, member := range recordMembers(r) {
		if member.padding >= 0 {
			p := r.Padding[member.padding]
			kind := bitfieldBaseType(p.Type)
			if alias := paddingKinds[member.padding]; alias != "" {
				kind = alias
			}
			name := ""
			if member.padding == padding {
				name = "value"
				if p.TypeAlignment != 0 {
					fmt.Fprintf(&source, "const unsigned long long dylib_bitfield_paddingbefore_%d_%d = __builtin_offsetof(struct %s{\n%s unsigned char marker;\n},marker);\n", record, padding, r.cAttributes(), body.String())
				}
			}
			fmt.Fprintf(&body, "%s %s:%d%s;\n", kind, name, p.Width, p.cAttributes())
		} else {
			j := member.field
			if bit := r.bitfieldAt(j); bit != nil {
				kind := bitfieldBaseType(r.Description.Fields[j].Type.Type)
				if alias := kinds[j]; alias != "" {
					kind = alias
				}
				fmt.Fprintf(&body, "%s f%d:%d%s;\n", kind, j, bit.Width, r.cFieldAttributes(j))
			} else {
				fmt.Fprintf(&body, "__typeof__(((%s *)0)->%s) f%d%s;\n", r.Name, r.Description.Fields[j].Name, j, r.cFieldAttributes(j))
			}
		}
	}
	p := r.Padding[padding]
	probe := fmt.Sprintf("dylib_padding_%d_%d_type", record, padding)
	fmt.Fprintf(&source, "typedef struct %s{\n%s} %s;\nconst %s dylib_bitfield_padding_%d_%d = {.value=%s};\n", r.cAttributes(), body.String(), probe, probe, record, padding, bitfieldValue(p.Type, p.Width))
	// Naming a field must not silently shift the ordinary members whose lane
	// classes the observation will be compared with.
	for i, field := range r.Description.Fields {
		if r.bitfieldAt(i) == nil {
			fmt.Fprintf(&source, "_Static_assert(__builtin_offsetof(%s,f%d)==__builtin_offsetof(%s,%s),\"unnamed probe moved a field\");\n", probe, i, r.Name, field.Name)
		}
	}
	if r.MaxFieldAlignment != 0 {
		fmt.Fprint(&source, "#pragma pack(pop)\n")
	}
	return source.String()
}
