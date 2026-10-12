package clang

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// FieldDecl only carries a type spelling in Clang's JSON AST. Ask for compiler
// type nodes rather than interpreting C function declarator strings. The same
// probes evaluate member alignment for type-based _Alignas declarations.
var errRecordFieldTypes = errors.New("clang: record fields require compiler type probes")

type recordField struct{ record, field string }

type recordFieldProbe struct {
	Type      astNode
	Alignment uint64
}

func recordFieldProbes(data []byte) (string, map[string]recordField, error) {
	var root astNode
	if err := json.Unmarshal(data, &root); err != nil {
		return "", nil, err
	}
	r := newTypeResolver(root, nil, nil)
	var names []string
	for id, name := range r.spellings {
		if r.nodes[id].CompleteDefinition && r.nodes[id].TagUsed == "struct" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	keys := make(map[string]recordField)
	var source strings.Builder
	for _, name := range names {
		for _, child := range r.nodes[r.names[name]].Inner {
			if child.Kind != "FieldDecl" || child.IsBitfield || !cIdentifier(child.Name) {
				continue
			}
			if len(keys) >= maxDescriptionNodes {
				return "", nil, fmt.Errorf("clang: too many record field probes")
			}
			probe := fmt.Sprintf("dylib_field_probe_%d", len(keys))
			keys[probe] = recordField{name, child.Name}
			fmt.Fprintf(&source, "typedef __typeof__(((%s *)0)->%s) %s;\n", name, child.Name, probe)
			fmt.Fprintf(&source, "enum { %s_alignment = __alignof__(((%s *)0)->%s) };\n", probe, name, child.Name)
			if source.Len() > maxHeaderSize {
				return "", nil, fmt.Errorf("clang: record field probes exceed source limit")
			}
		}
	}
	return source.String(), keys, nil
}

func decodeFieldProbes(data []byte, keys map[string]recordField) (map[recordField]recordFieldProbe, error) {
	var root astNode
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	fields := make(map[recordField]recordFieldProbe)
	for _, node := range root.Inner {
		if key, ok := keys[node.Name]; ok && node.Kind == "TypedefDecl" {
			probe := fields[key]
			probe.Type = node
			fields[key] = probe
		}
		if node.Kind != "EnumDecl" {
			continue
		}
		for _, value := range node.Inner {
			name, hasAlignment := strings.CutSuffix(value.Name, "_alignment")
			key, ok := keys[name]
			if !hasAlignment || !ok || value.Kind != "EnumConstantDecl" {
				continue
			}
			count, ok := constantValue(value)
			if !ok || count <= 0 || !validAlignment(uint64(count)) {
				return nil, fmt.Errorf("clang: invalid compiler field alignment %s", value.Name)
			}
			probe := fields[key]
			probe.Alignment = uint64(count)
			fields[key] = probe
		}
	}
	if len(fields) != len(keys) {
		return nil, fmt.Errorf("clang: missing compiler record field types")
	}
	for _, probe := range fields {
		if probe.Type.Kind != "TypedefDecl" || probe.Alignment == 0 {
			return nil, fmt.Errorf("clang: incomplete compiler record field probe")
		}
	}
	return fields, nil
}

func (r *typeResolver) describeField(name string, field astNode, depth int) (abi.TypeDesc, error) {
	key := recordField{name, field.Name}
	if probe, ok := r.fieldProbes[key]; ok {
		effective, err := r.effectiveType(probe.Type)
		if err != nil {
			return abi.TypeDesc{}, err
		}
		var lengths []int
		for effective.Kind == "ConstantArrayType" && len(effective.Inner) == 1 {
			if effective.Size < 1 || effective.Size > 65536 || len(lengths) >= 8 {
				return abi.TypeDesc{}, fmt.Errorf("unsupported function-pointer array shape")
			}
			lengths = append(lengths, effective.Size)
			effective, err = r.effectiveType(effective.Inner[0])
			if err != nil {
				return abi.TypeDesc{}, err
			}
		}
		pointee, layers, err := r.pointerPointee(effective)
		if err != nil {
			return abi.TypeDesc{}, err
		}
		if layers != 0 && (pointee.Kind == "FunctionProtoType" || pointee.Kind == "FunctionNoProtoType") {
			if r.fieldPointers == nil {
				r.fieldPointers = make(map[recordField]astNode)
			}
			r.fieldPointers[key] = effective
			description := abi.TypeDesc{Type: abi.Pointer}
			for i := len(lengths) - 1; i >= 0; i-- {
				element := description
				description = abi.TypeDesc{Type: abi.Array, Len: lengths[i], Elem: &element}
			}
			return description, description.Validate()
		}
	}
	description, err := r.describe(field.Type.QualType, false, depth)
	if err != nil && r.fieldProbes == nil {
		return abi.TypeDesc{}, errRecordFieldTypes
	}
	return description, err
}

func (r *typeResolver) resolveFieldPointers(target Target) error {
	// Physical descriptions are complete first. A callback may accept its own
	// enclosing record without recursively embedding its prototype in TypeDesc.
	// Resolving a callback can discover more records, so inspect the growing list.
	for i := 0; i < len(r.records); i++ {
		name, fields := r.records[i].Name, r.records[i].Description.Fields
		for j, field := range fields {
			node, ok := r.fieldPointers[recordField{name, field.Name}]
			if !ok {
				continue
			}
			_, pointer, err := r.boundaryType(node, target, 0)
			if err != nil {
				return fmt.Errorf("clang: %s.%s: %w", name, field.Name, err)
			}
			if pointer == nil {
				return fmt.Errorf("clang: missing record field prototype")
			}
			pointer.Position = j
			r.records[i].FunctionPointers = append(r.records[i].FunctionPointers, *pointer)
		}
	}
	return nil
}
