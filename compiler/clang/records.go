package clang

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

const maxRecords = 256
const maxDescriptionNodes = 65536

// Record contains a usable C type spelling, logical fields, and the compiler's
// target-specific storage layout. It describes an ordinary struct, not ownership.
type Record struct {
	Name        string
	Description abi.TypeDesc
	Layout      abi.Layout
}

// LookupRecord returns an independent snapshot for constructing logical values.
// This is metadata inspection; ForHost validates the native backend before calls.
func (h *Header) LookupRecord(name string) (Record, error) {
	if h != nil {
		if err := h.validateDescriptions(); err != nil {
			return Record{}, err
		}
		for _, record := range h.Records {
			if record.Name == name {
				if err := validRecordLayout(record); err != nil {
					return Record{}, err
				}
				record.Description = record.Description.Clone()
				record.Layout.Offsets = append([]uint64(nil), record.Layout.Offsets...)
				return record, nil
			}
		}
	}
	return Record{}, fmt.Errorf("clang: no record for %q", name)
}

func consumeDescription(d abi.TypeDesc, budget *int, depth int) error {
	if depth > 8 {
		return fmt.Errorf("native type nesting exceeds 8 levels")
	}
	*budget -= 1
	if *budget < 0 {
		return fmt.Errorf("clang: descriptions exceed %d nodes", maxDescriptionNodes)
	}
	for _, field := range d.Fields {
		if err := consumeDescription(field.Type, budget, depth+1); err != nil {
			return err
		}
	}
	if d.Elem != nil {
		return consumeDescription(*d.Elem, budget, depth+1)
	}
	return nil
}

func (h *Header) validateDescriptions() error {
	if h == nil || len(h.Records) > maxRecords || len(h.Functions) > maxFunctions {
		return fmt.Errorf("clang: declaration count exceeds supported limits")
	}
	budget := maxDescriptionNodes
	for _, function := range h.Functions {
		s := function.Signature
		if len(s.Args) > 256 {
			return fmt.Errorf("clang: at most 256 declared parameters are supported")
		}
		for _, description := range s.ArgTypes {
			if err := consumeDescription(description, &budget, 0); err != nil {
				return err
			}
		}
		if s.ResultType != nil {
			if err := consumeDescription(*s.ResultType, &budget, 0); err != nil {
				return err
			}
		}
		budget -= len(s.Args) + 1
		if budget < 0 {
			return fmt.Errorf("clang: descriptions exceed %d nodes", maxDescriptionNodes)
		}
		if err := s.Validate(); err != nil {
			return err
		}
	}
	for _, record := range h.Records {
		if record.Name == "" || record.Description.Type != abi.Struct {
			return fmt.Errorf("clang: invalid record description")
		}
		if err := consumeDescription(record.Description, &budget, 0); err != nil {
			return err
		}
		if err := record.Description.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func validRecordLayout(record Record) error {
	l := record.Layout
	if l.Size == 0 || l.Size > 65536 || l.Alignment == 0 || l.Alignment > l.Size || l.Alignment&(l.Alignment-1) != 0 || l.Size%l.Alignment != 0 || len(l.Offsets) != len(record.Description.Fields) {
		return fmt.Errorf("clang: invalid compiler layout for %s", record.Name)
	}
	for i, offset := range l.Offsets {
		if offset >= l.Size || i == 0 && offset != 0 || i > 0 && offset <= l.Offsets[i-1] {
			return fmt.Errorf("clang: invalid compiler offsets for %s", record.Name)
		}
	}
	return nil
}

func (h *Header) validateHostRecords(signature abi.Signature) error {
	var check func(abi.TypeDesc) error
	check = func(d abi.TypeDesc) error {
		if d.Type == abi.Struct {
			found := false
			var native abi.Layout
			for _, record := range h.Records {
				if !reflect.DeepEqual(record.Description, d) {
					continue
				}
				if err := validRecordLayout(record); err != nil {
					return err
				}
				if !found {
					var err error
					native, err = abi.LayoutOf(d, signature.Convention)
					if err != nil {
						return err
					}
					found = true
				}
				if !reflect.DeepEqual(native, record.Layout) {
					return fmt.Errorf("clang: compiler/backend layout mismatch for %s", record.Name)
				}
			}
			if !found {
				return fmt.Errorf("clang: missing compiler record layout")
			}
		}
		for _, field := range d.Fields {
			if err := check(field.Type); err != nil {
				return err
			}
		}
		if d.Elem != nil {
			return check(*d.Elem)
		}
		return nil
	}
	if err := check(signature.ReturnType()); err != nil {
		return err
	}
	for i := range signature.Args {
		if err := check(signature.ArgumentType(i)); err != nil {
			return err
		}
	}
	return nil
}

func (h *Header) recordProbes() string {
	var source strings.Builder
	fmt.Fprintln(&source, "enum {")
	for i, record := range h.Records {
		fmt.Fprintf(&source, "dylib_record_%d_size=sizeof(%s),\ndylib_record_%d_align=_Alignof(%s),\n", i, record.Name, i, record.Name)
		for j, field := range record.Description.Fields {
			fmt.Fprintf(&source, "dylib_record_%d_offset_%d=__builtin_offsetof(%s,%s),\n", i, j, record.Name, field.Name)
		}
	}
	fmt.Fprintln(&source, "};")
	return source.String()
}

func (h *Header) decodeRecordLayouts(data []byte) error {
	var root astNode
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("clang: invalid layout AST: %w", err)
	}
	if root.Kind != "TranslationUnitDecl" {
		return fmt.Errorf("clang: expected a layout translation unit")
	}
	values := make(map[string]int)
	for _, node := range root.Inner {
		if node.Kind != "EnumDecl" {
			continue
		}
		for _, field := range node.Inner {
			if !strings.HasPrefix(field.Name, "dylib_record_") {
				continue
			}
			value, ok := constantValue(field)
			if !ok || value < 0 {
				return fmt.Errorf("clang: missing evaluated layout probe %s", field.Name)
			}
			values[field.Name] = value
		}
	}
	get := func(name string) (uint64, error) {
		value, ok := values[name]
		if !ok {
			return 0, fmt.Errorf("clang: missing layout probe %s", name)
		}
		return uint64(value), nil
	}
	for i := range h.Records {
		var layout abi.Layout
		var err error
		if layout.Size, err = get(fmt.Sprintf("dylib_record_%d_size", i)); err != nil {
			return err
		}
		if layout.Alignment, err = get(fmt.Sprintf("dylib_record_%d_align", i)); err != nil {
			return err
		}
		for j := range h.Records[i].Description.Fields {
			offset, err := get(fmt.Sprintf("dylib_record_%d_offset_%d", i, j))
			if err != nil {
				return err
			}
			layout.Offsets = append(layout.Offsets, offset)
		}
		h.Records[i].Layout = layout
		if err := validRecordLayout(h.Records[i]); err != nil {
			return err
		}
	}
	return nil
}
