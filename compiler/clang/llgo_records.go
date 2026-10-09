package clang

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

type directTypes struct {
	records []Record
	prefix  string
	fields  [][]string
}

func directName(name string) string {
	if name[0] >= 'a' && name[0] <= 'z' {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	if name[0] == '_' {
		return "X" + name
	}
	return name
}

func newDirectTypes(records []Record, prefix string) (*directTypes, error) {
	t := &directTypes{records: records, prefix: prefix}
	for _, record := range records {
		seen := make(map[string]bool)
		var names []string
		for _, field := range record.Description.Fields {
			if !cIdentifier(field.Name) {
				return nil, fmt.Errorf("clang: unsupported direct field name %q", field.Name)
			}
			name := directName(field.Name)
			if seen[name] {
				return nil, fmt.Errorf("clang: ambiguous generated field %s in %s", name, record.Name)
			}
			seen[name] = true
			names = append(names, name)
		}
		t.fields = append(t.fields, names)
	}
	// Validate all member types before producing any source.
	for _, record := range records {
		for _, field := range record.Description.Fields {
			if _, err := t.goType(field.Type); err != nil {
				return nil, err
			}
		}
	}
	return t, nil
}

func (t *directTypes) recordName(index int) string {
	return fmt.Sprintf("%sRecord%d", t.prefix, index)
}

func (t *directTypes) goType(d abi.TypeDesc) (string, error) {
	switch d.Type {
	case abi.Struct:
		for i, record := range t.records {
			if reflect.DeepEqual(record.Description, d) {
				return t.recordName(i), nil
			}
		}
		return "", fmt.Errorf("clang: missing compiler record for direct type")
	case abi.Array:
		element, err := t.goType(*d.Elem)
		return fmt.Sprintf("[%d]%s", d.Len, element), err
	case abi.Pointer:
		if d.Elem != nil {
			element, err := t.goType(*d.Elem)
			if err != nil {
				return "", err
			}
			if element == "" {
				return "", fmt.Errorf("clang: void pointees require opaque native pointers")
			}
			return "*" + element, nil
		}
	}
	return directGoType(d)
}

func (t *directTypes) writeRecords(output *bytes.Buffer) {
	for i, record := range t.records {
		fmt.Fprintf(output, "\n// %s describes %s; pointer fields are caller-managed native addresses.\n//llgo:type C\ntype %s struct {\n", t.recordName(i), record.Name, t.recordName(i))
		for j, field := range record.Description.Fields {
			kind, _ := t.goType(field.Type)
			fmt.Fprintf(output, "%s %s\n", t.fields[i][j], kind)
		}
		fmt.Fprint(output, "}\n")
	}
}

func (t *directTypes) writeLayoutChecks(output *bytes.Buffer) {
	for i, record := range t.records {
		name, layout := t.recordName(i), record.Layout
		fmt.Fprintf(output, "if uint64(unsafe.Sizeof(%s{})) != %d || uint64(unsafe.Alignof(%s{})) != %d", name, layout.Size, name, layout.Alignment)
		for j, offset := range layout.Offsets {
			fmt.Fprintf(output, " || uint64(unsafe.Offsetof(%s{}.%s)) != %d", name, t.fields[i][j], offset)
		}
		fmt.Fprintf(output, " { return nil,fmt.Errorf(%q) }\n", "clang: compiler/llgo layout mismatch for "+record.Name)
	}
}
