package clang

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ramblelabs/dylib-go/abi"
)

type typeResolver struct {
	target         Target
	nominalRecords bool
	defaultPacking *uint64
	aliases        map[string]string
	enums          map[string]*enumType
	aliasNodes     map[string]astNode
	sizes          map[string]int
	nodes          map[string]astNode
	names          map[string]string
	spellings      map[string]string
	resolved       map[string]abi.TypeDesc
	building       map[string]bool
	records        []Record
	fieldProbes    map[recordField]recordFieldProbe
	fieldPointers  map[recordField]astNode
	packings       map[string]uint64
}

func newTypeResolver(root astNode, aliases map[string]string, sizes map[string]int) *typeResolver {
	r := &typeResolver{aliases: aliases, aliasNodes: make(map[string]astNode), sizes: sizes, nodes: make(map[string]astNode), names: make(map[string]string), spellings: make(map[string]string), resolved: make(map[string]abi.TypeDesc), building: make(map[string]bool)}
	r.enums = indexEnums(root)
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
		if node.Kind == "TypedefDecl" {
			r.aliasNodes[node.Name] = node
		}
	}
	for _, node := range root.Inner {
		if node.Kind != "TypedefDecl" {
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
		r.names[node.Name] = id
		// Aligned aliases share their body's fields and calling ABI, but must
		// not replace its spelling in layout probes. Alias alignment does not
		// round the body's size or change its natural alignment.
		if r.recordTypedefAttributes(node) {
			continue
		}
		r.names[spelling] = id
		if r.spellings[id] == "" || strings.HasPrefix(r.spellings[id], "struct ") {
			r.spellings[id] = node.Name
		}
	}
	// Anonymous bodies may have no unaligned spelling. Retain a written alias
	// for field inspection and reconstruct its canonical body in compiler probes.
	for _, node := range root.Inner {
		if node.Kind == "TypedefDecl" {
			if id := r.names[node.Name]; id != "" && r.spellings[id] == "" {
				r.spellings[id] = node.Name
			}
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
	alignment, err := r.typedefAlignment(spelling, depth)
	if err != nil {
		return abi.TypeDesc{}, err
	}
	if id := r.names[spelling]; id != "" {
		d, err := r.record(id, depth)
		if err != nil {
			return d, err
		}
		if alignment != 0 {
			r.rememberRecordTypedef(id, spelling, alignment)
		}
		return d, nil
	}
	if e := r.enums[spelling]; e != nil {
		return r.describeEnum(e)
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
			if _, err := r.typedefAlignment(base, depth+1); err != nil {
				return abi.TypeDesc{}, err
			}
			alias, ok := r.aliases[base]
			if !ok {
				break
			}
			base = unqualified(alias)
		}
		if id := r.names[base]; id != "" && r.nodes[id].TagUsed == "struct" {
			element, err := r.describe(base, false, depth+1)
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
	if r.defaultPacking == nil {
		return abi.TypeDesc{}, errDefaultPacking
	}
	if len(r.records) >= maxRecords {
		return abi.TypeDesc{}, fmt.Errorf("at most %d records are supported", maxRecords)
	}
	r.building[id] = true
	defer delete(r.building, id)
	d := abi.TypeDesc{Type: abi.Struct}
	if r.nominalRecords {
		d.RecordID = name
	}
	packed := false
	packing := *r.defaultPacking
	aligned := uint64(0)
	var fieldAlignments []FieldAlignment
	var fieldTypeAlignments []TypedefFieldAlignment
	var packedFields []int
	var bitfields []Bitfield
	var padding []BitfieldPadding
	for _, child := range node.Inner {
		if child.Kind == "MaxFieldAlignmentAttr" {
			pragma := r.packings[name]
			if pragma == 0 {
				return abi.TypeDesc{}, errRecordPacking
			}
			// MSVC ignores oversized pragmas and retains the global default.
			// Preserve existing request metadata when no global default exists.
			if packing == 0 || !r.target.msLayout() || pragma <= uint64(r.target.PointerSize) {
				packing = pragma
			}
			continue
		}
		if child.Kind == "PackedAttr" {
			packed = true
			continue
		}
		if child.Kind == "AlignedAttr" {
			value, err := r.recordAlignment(child)
			if err != nil {
				return abi.TypeDesc{}, fmt.Errorf("%s: %w", name, err)
			}
			if value > aligned {
				aligned = value
			}
			continue
		}
		if strings.HasSuffix(child.Kind, "Attr") {
			return abi.TypeDesc{}, fmt.Errorf("unsupported record attribute %s", child.Kind)
		}
		if child.Kind != "FieldDecl" {
			continue
		}
		unnamed := child.IsBitfield && child.Name == ""
		if !unnamed && !cIdentifier(child.Name) {
			return abi.TypeDesc{}, fmt.Errorf("unnamed record fields are unsupported")
		}
		fieldAligned := uint64(0)
		fieldPacked := false
		for _, attr := range child.Inner {
			if attr.Kind == "PackedAttr" {
				fieldPacked = true
				continue
			}
			if attr.Kind == "AlignedAttr" {
				value, err := r.memberAlignment(name, child, attr)
				if err != nil {
					return abi.TypeDesc{}, fmt.Errorf("%s.%s: %w", name, child.Name, err)
				}
				if value > fieldAligned {
					fieldAligned = value
				}
				continue
			}
			if strings.HasSuffix(attr.Kind, "Attr") {
				return abi.TypeDesc{}, fmt.Errorf("unsupported field attribute %s", attr.Kind)
			}
		}
		// Recursive record pointers are ordinary addresses. Only function root
		// struct pointers receive typed temporary-pointee descriptions.
		field, err := r.describeField(name, child, depth+1)
		if err != nil {
			return abi.TypeDesc{}, fmt.Errorf("%s.%s: %w", name, child.Name, err)
		}
		typeAlignments, err := r.typedefAlignments(child.Type.QualType, depth+1)
		if err != nil {
			return abi.TypeDesc{}, fmt.Errorf("%s.%s: %w", name, child.Name, err)
		}
		if !child.IsBitfield {
			for _, alignment := range typeAlignments {
				alignment.Position = len(d.Fields)
				fieldTypeAlignments = append(fieldTypeAlignments, alignment)
			}
		}
		if child.IsBitfield {
			width, ok := constantValue(child)
			bits := bitfieldTypeBits(field.Type)
			if !ok || bits == 0 || width < 0 || !unnamed && width == 0 || width > bits || field.Elem != nil {
				return abi.TypeDesc{}, fmt.Errorf("unsupported bitfield %s.%s", name, child.Name)
			}
			if unnamed {
				p := BitfieldPadding{Before: len(d.Fields), Type: field.Type, Width: width, Packed: fieldPacked, Aligned: fieldAligned}
				if len(typeAlignments) != 0 {
					p.TypeAlignment = typeAlignments[0].Aligned
				}
				padding = append(padding, p)
			} else {
				bit := Bitfield{Position: len(d.Fields), Width: width}
				if len(typeAlignments) != 0 {
					bit.TypeAlignment = typeAlignments[0].Aligned
				}
				bitfields = append(bitfields, bit)
			}
		}
		if !unnamed {
			if fieldPacked {
				packedFields = append(packedFields, len(d.Fields))
			}
			if fieldAligned != 0 {
				fieldAlignments = append(fieldAlignments, FieldAlignment{Position: len(d.Fields), Aligned: fieldAligned})
			}
			d.Fields = append(d.Fields, abi.Field{Name: child.Name, Type: field})
		}
		if len(d.Fields)+len(padding) > 32 {
			return abi.TypeDesc{}, fmt.Errorf("structs require at most 32 named/padding fields")
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
	r.records = append(r.records, Record{Name: name, Description: d, Packed: packed, MaxFieldAlignment: packing, PackedFields: packedFields, Aligned: aligned, FieldAlignments: fieldAlignments, FieldTypeAlignments: fieldTypeAlignments, Bitfields: bitfields, Padding: padding})
	if alignment, err := r.typedefAlignment(name, depth); err != nil {
		return abi.TypeDesc{}, err
	} else if alignment != 0 {
		r.rememberRecordTypedef(id, name, alignment)
	}
	return d, nil
}
