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
// type nodes rather than interpreting C function declarator strings.
var errRecordFieldTypes = errors.New("clang: record fields require compiler type probes")

type recordField struct{ record, field string }

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
			if source.Len() > maxHeaderSize {
				return "", nil, fmt.Errorf("clang: record field probes exceed source limit")
			}
		}
	}
	return source.String(), keys, nil
}

func decodeFieldTypes(data []byte, keys map[string]recordField) (map[recordField]astNode, error) {
	var root astNode
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	fields := make(map[recordField]astNode)
	for _, node := range root.Inner {
		if key, ok := keys[node.Name]; ok && node.Kind == "TypedefDecl" {
			fields[key] = node
		}
	}
	if len(fields) != len(keys) {
		return nil, fmt.Errorf("clang: missing compiler record field types")
	}
	return fields, nil
}

func (r *typeResolver) describeField(name string, field astNode, depth int) (abi.TypeDesc, error) {
	key := recordField{name, field.Name}
	if node, ok := r.fieldTypes[key]; ok {
		pointee, layers, err := r.pointerPointee(node)
		if err != nil {
			return abi.TypeDesc{}, err
		}
		if layers != 0 && (pointee.Kind == "FunctionProtoType" || pointee.Kind == "FunctionNoProtoType") {
			if r.fieldPointers == nil {
				r.fieldPointers = make(map[recordField]astNode)
			}
			r.fieldPointers[key] = node
			return abi.TypeDesc{Type: abi.Pointer}, nil
		}
	}
	description, err := r.describe(field.Type.QualType, false, depth)
	if err != nil && r.fieldTypes == nil {
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
