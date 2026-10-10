package clang

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// Public records use Go fields. Only independently constructed C values cross
// the cgo boundary, so their padding/alignment need not match Go storage.
type cgoTypes struct {
	*directTypes
	cPrefix string
}

func (t *cgoTypes) recordIndex(description abi.TypeDesc) (int, error) {
	name, err := t.directTypes.goType(description)
	if err != nil {
		return 0, err
	}
	for i := range t.records {
		if name == t.recordName(i) {
			return i, nil
		}
	}
	return 0, fmt.Errorf("clang: missing compiler record for cgo type")
}

func (t *cgoTypes) cRecordName(index int) string {
	return fmt.Sprintf("%s_record%d", t.cPrefix, index)
}

func (t *cgoTypes) cType(description abi.TypeDesc) (string, error) {
	if description.Type == abi.Struct {
		index, err := t.recordIndex(description)
		return t.cRecordName(index), err
	}
	if description.Type == abi.Pointer && description.Elem != nil && description.Elem.Type == abi.Struct {
		index, err := t.recordIndex(*description.Elem)
		return t.cRecordName(index) + " *", err
	}
	return cgoScalarType(description)
}

func (t *cgoTypes) goType(description abi.TypeDesc) (string, error) {
	if description.Type == abi.Struct {
		return t.directTypes.goType(description)
	}
	if description.Type == abi.Pointer && description.Elem != nil && description.Elem.Type == abi.Struct {
		return "unsafe.Pointer", nil
	}
	return directGoType(description)
}

func (t *cgoTypes) transport(description abi.TypeDesc) string {
	if description.Type == abi.Struct {
		name, _ := t.cType(description)
		return name
	}
	return cgoTransportType(description.Type)
}

func (t *cgoTypes) conversion(index int, toC bool) string {
	direction := "FromC"
	if toC {
		direction = "ToC"
	}
	return "_" + t.recordName(index) + direction
}

func (t *cgoTypes) cField(description abi.TypeDesc, name string) (string, error) {
	var suffix strings.Builder
	for description.Type == abi.Array {
		fmt.Fprintf(&suffix, "[%d]", description.Len)
		description = *description.Elem
	}
	if description.Type == abi.Pointer && description.Elem != nil {
		return "", fmt.Errorf("typed record fields require opaque native addresses")
	}
	kind, err := t.cType(description)
	return kind + " " + name + suffix.String(), err
}

func (t *cgoTypes) writeCRecords(output *bytes.Buffer) error {
	// Header values can be assembled by callers, so emit dependencies first even
	// when their metadata order differs from Clang's parse order.
	seen := make(map[int]bool)
	var emit func(int) error
	emit = func(i int) error {
		if seen[i] {
			return nil
		}
		seen[i] = true
		record := t.records[i]
		for _, field := range record.Description.Fields {
			description := field.Type
			for description.Type == abi.Array {
				description = *description.Elem
			}
			if description.Type == abi.Struct {
				index, err := t.recordIndex(description)
				if err != nil {
					return err
				}
				if err := emit(index); err != nil {
					return err
				}
			}
		}
		name := t.cRecordName(i)
		fmt.Fprint(output, "typedef struct {\n")
		for j, field := range record.Description.Fields {
			declaration, err := t.cField(field.Type, fmt.Sprintf("f%d", j))
			if err != nil {
				return fmt.Errorf("clang: %s.%s: %w", record.Name, field.Name, err)
			}
			fmt.Fprintf(output, "%s;\n", declaration)
		}
		fmt.Fprintf(output, "} %s;\nstatic uint64_t %s_layout(int32_t index) {\nswitch(index) {\ncase 0:return sizeof(%s);\ncase 1:return _Alignof(%s);\n", name, name, name, name)
		for j := range record.Description.Fields {
			fmt.Fprintf(output, "case %d:return offsetof(%s,f%d);\n", j+2, name, j)
		}
		fmt.Fprint(output, "default:return UINT64_MAX;\n}\n}\n")
		return nil
	}
	for i := range t.records {
		if err := emit(i); err != nil {
			return err
		}
	}
	return nil
}

func (t *cgoTypes) writeCLayoutChecks(output *bytes.Buffer) {
	for i, record := range t.records {
		name, layout := t.cRecordName(i), record.Layout
		fmt.Fprintf(output, "if uint64(C.%s_layout(0)) != %d || uint64(C.%s_layout(1)) != %d", name, layout.Size, name, layout.Alignment)
		for j, offset := range layout.Offsets {
			fmt.Fprintf(output, " || uint64(C.%s_layout(%d)) != %d", name, j+2, offset)
		}
		fmt.Fprintf(output, " { return nil,fmt.Errorf(%q) }\n", "clang: compiler/cgo layout mismatch for "+record.Name)
	}
}

func (t *cgoTypes) writeConversions(output *bytes.Buffer) error {
	for i, record := range t.records {
		for _, toC := range []bool{true, false} {
			from, to := t.recordName(i), "C."+t.cRecordName(i)
			if !toC {
				from, to = to, from
			}
			fmt.Fprintf(output, "\nfunc %s(value %s) (result %s) {\n", t.conversion(i, toC), from, to)
			for j, field := range record.Description.Fields {
				goField, cField := "."+t.fields[i][j], fmt.Sprintf(".f%d", j)
				destination, source := "result"+cField, "value"+goField
				if !toC {
					destination, source = "result"+goField, "value"+cField
				}
				if err := t.writeFieldCopy(output, field.Type, destination, source, toC, 0); err != nil {
					return err
				}
			}
			fmt.Fprint(output, "return result\n}\n")
		}
	}
	return nil
}

func (t *cgoTypes) writeFieldCopy(output *bytes.Buffer, description abi.TypeDesc, destination, source string, toC bool, depth int) error {
	if description.Type == abi.Array {
		index := fmt.Sprintf("i%d", depth)
		fmt.Fprintf(output, "for %s := range %d {\n", index, description.Len)
		if err := t.writeFieldCopy(output, *description.Elem, destination+"["+index+"]", source+"["+index+"]", toC, depth+1); err != nil {
			return err
		}
		fmt.Fprint(output, "}\n")
		return nil
	}
	var expression string
	if description.Type == abi.Struct {
		index, err := t.recordIndex(description)
		if err != nil {
			return err
		}
		expression = t.conversion(index, toC) + "(" + source + ")"
	} else if description.Type == abi.Pointer {
		if _, err := t.cType(description); err != nil {
			return err
		}
		expression = source
	} else {
		kind, err := t.goType(description)
		if toC {
			kind, err = t.cType(description)
			kind = "C." + kind
		}
		if err != nil {
			return err
		}
		// cgo names the native float/double types C.float and C.double.
		expression = kind + "(" + source + ")"
	}
	fmt.Fprintf(output, "%s = %s\n", destination, expression)
	return nil
}

func containsNativePointer(description abi.TypeDesc) bool {
	if description.Type == abi.Pointer {
		return true
	}
	if description.Elem != nil && containsNativePointer(*description.Elem) {
		return true
	}
	for _, field := range description.Fields {
		if containsNativePointer(field.Type) {
			return true
		}
	}
	return false
}
