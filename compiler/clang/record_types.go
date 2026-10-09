package clang

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

type typeResolver struct {
	aliases         map[string]string
	aliasAttributes map[string]string
	sizes           map[string]int
	nodes           map[string]astNode
	names           map[string]string
	spellings       map[string]string
	resolved        map[string]abi.TypeDesc
	building        map[string]bool
	records         []Record
}

func newTypeResolver(root astNode, aliases map[string]string, sizes map[string]int) *typeResolver {
	r := &typeResolver{aliases: aliases, aliasAttributes: make(map[string]string), sizes: sizes, nodes: make(map[string]astNode), names: make(map[string]string), spellings: make(map[string]string), resolved: make(map[string]abi.TypeDesc), building: make(map[string]bool)}
	var index func(astNode)
	index = func(node astNode) {
		if node.Kind != "RecordDecl" {
			return
		}
		r.nodes[node.ID] = node
		if node.CompleteDefinition && node.Name != "" {
			name := node.TagUsed + " " + node.Name
			r.names[name], r.spellings[node.ID] = node.ID, name
		}
		for _, child := range node.Inner {
			index(child)
		}
	}
	for _, node := range root.Inner {
		index(node)
	}
	for _, node := range root.Inner {
		if node.Kind != "TypedefDecl" {
			continue
		}
		for _, child := range node.Inner {
			if strings.HasSuffix(child.Kind, "Attr") {
				r.aliasAttributes[node.Name] = child.Kind
			}
		}
		if r.aliasAttributes[node.Name] != "" {
			continue
		}
		id := recordReference(node)
		record, ok := r.nodes[id]
		if !ok {
			continue
		}
		if !record.CompleteDefinition {
			id = r.names[record.TagUsed+" "+record.Name]
			record = r.nodes[id]
		}
		// A pointer/array typedef can contain a RecordType too; it is not an alias
		// for that record itself. Resolve those through their ordinary spelling.
		spelling := node.Type.spelling()
		if !record.CompleteDefinition || strings.ContainsAny(spelling, "*[]()") {
			continue
		}
		r.names[node.Name], r.names[spelling] = id, id
		if r.spellings[id] == "" || strings.HasPrefix(r.spellings[id], "struct ") {
			r.spellings[id] = node.Name
		}
	}
	return r
}

func recordReference(node astNode) string {
	if node.Kind == "RecordType" && node.Decl != nil {
		return node.Decl.ID
	}
	for _, child := range node.Inner {
		if id := recordReference(child); id != "" {
			return id
		}
	}
	return ""
}

func unqualified(spelling string) string {
	var words []string
	for _, word := range strings.Fields(strings.ReplaceAll(spelling, "*", " * ")) {
		switch word {
		case "const", "volatile", "restrict", "__restrict", "__restrict__":
		default:
			words = append(words, word)
		}
	}
	return strings.Join(words, " ")
}

func complexDescription(d abi.TypeDesc) bool {
	return d.Type == abi.Struct || d.Type == abi.Array || d.Elem != nil
}

func (r *typeResolver) describe(spelling string, typedPointers bool, depth int) (abi.TypeDesc, error) {
	if depth > 16 {
		return abi.TypeDesc{}, fmt.Errorf("C type nesting exceeds 16 levels")
	}
	spelling = unqualified(spelling)
	if attr := r.aliasAttributes[spelling]; attr != "" {
		return abi.TypeDesc{}, fmt.Errorf("unsupported typedef attribute %s on %s", attr, spelling)
	}
	if id := r.names[spelling]; id != "" {
		return r.record(id, depth)
	}
	if alias, ok := r.aliases[spelling]; ok {
		return r.describe(alias, typedPointers, depth+1)
	}
	if open := strings.IndexByte(spelling, '['); open >= 0 && !strings.ContainsAny(spelling, "()") {
		base, suffix := strings.TrimSpace(spelling[:open]), spelling[open:]
		var lengths []int
		for suffix != "" {
			if suffix[0] != '[' {
				return abi.TypeDesc{}, fmt.Errorf("unsupported C array %q", spelling)
			}
			close := strings.IndexByte(suffix, ']')
			if close < 0 {
				return abi.TypeDesc{}, fmt.Errorf("unsupported C array %q", spelling)
			}
			length, err := strconv.Atoi(strings.TrimSpace(suffix[1:close]))
			if err != nil || length < 1 || length > 65536 {
				return abi.TypeDesc{}, fmt.Errorf("unsupported C array length in %q", spelling)
			}
			lengths = append(lengths, length)
			suffix = strings.TrimSpace(suffix[close+1:])
		}
		d, err := r.describe(base, false, depth+1)
		if err != nil {
			return abi.TypeDesc{}, err
		}
		for i := len(lengths) - 1; i >= 0; i-- {
			element := d
			d = abi.TypeDesc{Type: abi.Array, Len: lengths[i], Elem: &element}
		}
		return d, d.Validate()
	}
	if typedPointers && strings.Count(spelling, "*") == 1 && strings.HasSuffix(spelling, " *") {
		base := strings.TrimSpace(strings.TrimSuffix(spelling, " *"))
		for i := 0; i < 16 && r.names[base] == ""; i++ {
			if attr := r.aliasAttributes[base]; attr != "" {
				return abi.TypeDesc{}, fmt.Errorf("unsupported typedef attribute %s on %s", attr, base)
			}
			alias, ok := r.aliases[base]
			if !ok {
				break
			}
			base = unqualified(alias)
		}
		if id := r.names[base]; id != "" && r.nodes[id].TagUsed == "struct" {
			element, err := r.record(id, depth+1)
			if err != nil {
				return abi.TypeDesc{}, err
			}
			d := abi.TypeDesc{Type: abi.Pointer, Elem: &element}
			return d, d.Validate()
		}
	}
	typ, err := scalarType(spelling, r.aliases, r.sizes, depth)
	return abi.TypeDesc{Type: typ}, err
}

func (r *typeResolver) record(id string, depth int) (abi.TypeDesc, error) {
	if d, ok := r.resolved[id]; ok {
		return d, nil
	}
	if r.building[id] {
		return abi.TypeDesc{}, fmt.Errorf("recursive by-value record")
	}
	node := r.nodes[id]
	name := r.spellings[id]
	if node.TagUsed != "struct" || !node.CompleteDefinition || name == "" {
		return abi.TypeDesc{}, fmt.Errorf("unsupported C type: incomplete, union or inaccessible record")
	}
	if len(r.records) >= maxRecords {
		return abi.TypeDesc{}, fmt.Errorf("at most %d records are supported", maxRecords)
	}
	r.building[id] = true
	defer delete(r.building, id)
	d := abi.TypeDesc{Type: abi.Struct}
	for _, child := range node.Inner {
		if strings.HasSuffix(child.Kind, "Attr") {
			return abi.TypeDesc{}, fmt.Errorf("unsupported record attribute %s", child.Kind)
		}
		if child.Kind != "FieldDecl" {
			continue
		}
		if child.IsBitfield || !cIdentifier(child.Name) {
			return abi.TypeDesc{}, fmt.Errorf("bitfields and unnamed record fields are unsupported")
		}
		for _, attr := range child.Inner {
			if strings.HasSuffix(attr.Kind, "Attr") {
				return abi.TypeDesc{}, fmt.Errorf("unsupported field attribute %s", attr.Kind)
			}
		}
		// Recursive record pointers are ordinary addresses. Only function root
		// struct pointers receive typed temporary-pointee descriptions.
		field, err := r.describe(child.Type.QualType, false, depth+1)
		if err != nil {
			return abi.TypeDesc{}, fmt.Errorf("%s.%s: %w", name, child.Name, err)
		}
		d.Fields = append(d.Fields, abi.Field{Name: child.Name, Type: field})
		if len(d.Fields) > 32 {
			return abi.TypeDesc{}, fmt.Errorf("structs require 1 to 32 fields")
		}
	}
	if err := d.Validate(); err != nil {
		return abi.TypeDesc{}, err
	}
	// Nested construction can add records too, so check again before publication.
	if len(r.records) >= maxRecords {
		return abi.TypeDesc{}, fmt.Errorf("at most %d records are supported", maxRecords)
	}
	r.resolved[id] = d
	r.records = append(r.records, Record{Name: name, Description: d})
	return d, nil
}
