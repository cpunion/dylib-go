package clang

import (
	"fmt"
	"strings"
)

// Name may be the only accessible aligned alias of an anonymous body. Its
// alignment must not stand in for the natural alignment of that body.
func (r Record) namedTypedefAlignment() uint64 {
	for _, alias := range r.TypedefAlignments {
		if alias.Name == r.Name {
			return alias.Aligned
		}
	}
	return 0
}

func (r Record) bodyProbeName(index int) string {
	if r.namedTypedefAlignment() != 0 {
		return fmt.Sprintf("dylib_record_body_%d", index)
	}
	return r.Name
}

func (h *Header) recordBodyProbes() string {
	var source strings.Builder
	for i, r := range h.Records {
		if r.namedTypedefAlignment() == 0 {
			continue
		}
		name := r.bodyProbeName(i)
		aliases, kinds, paddingKinds := r.bitfieldTypedefProbes(name + "_bittypedef")
		source.WriteString(aliases)
		// Override the header's final packing state, including unclosed pragmas.
		// A zero cap restores the producer's compiler-default packing policy.
		fmt.Fprintf(&source, "#pragma pack(push,%d)\ntypedef struct %s{\n", r.MaxFieldAlignment, r.cAttributes())
		for _, member := range recordMembers(r) {
			if member.padding >= 0 {
				p := r.Padding[member.padding]
				kind := bitfieldBaseType(p.Type)
				if alias := paddingKinds[member.padding]; alias != "" {
					kind = alias
				}
				fmt.Fprintf(&source, "%s :%d%s;\n", kind, p.Width, p.cAttributes())
				continue
			}
			j := member.field
			field := r.Description.Fields[j]
			if bit := r.bitfieldAt(j); bit != nil {
				kind := bitfieldBaseType(field.Type.Type)
				if alias := kinds[j]; alias != "" {
					kind = alias
				}
				fmt.Fprintf(&source, "%s %s:%d%s;\n", kind, field.Name, bit.Width, r.cFieldAttributes(j))
			} else {
				fmt.Fprintf(&source, "__typeof__(((%s*)0)->%s) %s%s;\n", r.Name, field.Name, field.Name, r.cFieldAttributes(j))
			}
		}
		fmt.Fprintf(&source, "} %s;\n#pragma pack(pop)\n", name)
	}
	return source.String()
}
