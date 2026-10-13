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
// target-specific storage layout. It does not describe pointer ownership.
type Record struct {
	Name        string
	Description abi.TypeDesc
	Layout      abi.Layout
	// Packed requests the C compiler's packed attribute. Description remains a
	// logical field model; only compiled C adapters may pass this record by value.
	Packed bool
	// MaxFieldAlignment retains a pragma/default-compiler member-alignment cap
	// in bytes; the independently measured native layout remains authoritative.
	// Zero is absent; only compiled C adapters preserve this native layout.
	MaxFieldAlignment uint64
	// MemberAlignments retains compiler-observed storage alignment for each
	// pragma-packed member. Ordinary values are reconciled with the record base
	// and offset. MS bitfields retain alignment only at allocation-unit starts;
	// other named bitfields use zero. These are observations, not source requests.
	MemberAlignments []uint64
	// PackedFields indexes explicitly packed named members, including bitfields.
	// Public values remain ordinary Go fields and require a compiled C adapter.
	PackedFields []int
	// Aligned preserves a compiler-evaluated record alignment request in bytes;
	// zero is absent. Omitted GNU arguments use the target maximum alignment.
	// FieldAlignments preserves explicit alignment on named members. These
	// declarations require compiled C adapters, never a Go memory view.
	Aligned         uint64
	FieldAlignments []FieldAlignment
	// IndirectArgument is compiler-observed MSVC i386 argument lowering for an
	// aligned record. Generated bridges reconcile this with a MinGW host.
	IndirectArgument bool
	// IndirectResult records an MSVC i386 hidden result pointer requiring a
	// compiled C adapter for GCC fastcall interoperability.
	IndirectResult bool
	// Bitfields retains the width and native byte mask of each named bitfield.
	// Layout.Offsets stores its first byte. Description remains logical fields.
	Bitfields []Bitfield
	// Padding retains unnamed/zero-width C bitfields in declaration order.
	Padding []BitfieldPadding
	// FunctionPointers describes pointer fields and fixed arrays of entries.
	// Position is the field index; array shape remains in Description. All
	// elements share the prototype and have caller-managed address lifetimes.
	FunctionPointers []FunctionPointer
}

// LookupFunctionPointer returns an independent prototype for a named field or
// the common element prototype of its fixed array.
// Validate the header with ForHostRecord before calling addresses or callbacks.
func (r Record) LookupFunctionPointer(field string) (FunctionPointer, error) {
	if err := (&Header{Records: []Record{r}}).validateDescriptions(); err != nil {
		return FunctionPointer{}, err
	}
	for _, pointer := range r.FunctionPointers {
		if r.Description.Fields[pointer.Position].Name == field {
			return pointer.clone(), nil
		}
	}
	return FunctionPointer{}, fmt.Errorf("clang: no function pointer field %q in %s", field, r.Name)
}

// ForHostRecord checks native record and callback-prototype layouts before
// returning an independent snapshot. Addresses and registrations are borrowed.
func (h *Header) ForHostRecord(name string) (Record, error) {
	if h == nil {
		return Record{}, fmt.Errorf("clang: nil header")
	}
	if err := h.Target.CheckHost(); err != nil {
		return Record{}, err
	}
	record, err := h.LookupRecord(name)
	if err != nil {
		return Record{}, err
	}
	if err := h.validateHostRecords(abi.Signature{Result: abi.Struct, ResultType: &record.Description}); err != nil {
		return Record{}, err
	}
	return record, nil
}

func recordFieldSignature(record Record) abi.Signature {
	s := abi.Signature{}
	for _, field := range record.Description.Fields {
		description := field.Type
		for description.Type == abi.Array {
			description = *description.Elem
		}
		s.Args = append(s.Args, description.Type)
		s.ArgTypes = append(s.ArgTypes, description)
	}
	return s
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
				record.FunctionPointers = cloneFunctionPointers(record.FunctionPointers)
				record.Bitfields = cloneBitfields(record.Bitfields)
				record.Padding = append([]BitfieldPadding(nil), record.Padding...)
				record.FieldAlignments = append([]FieldAlignment(nil), record.FieldAlignments...)
				record.MemberAlignments = append([]uint64(nil), record.MemberAlignments...)
				record.PackedFields = append([]int(nil), record.PackedFields...)
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
		if err := validateSignature(function.Signature, &budget); err != nil {
			return err
		}
		if err := validateFunctionPointers(function.Signature, function.FunctionPointers, &budget, 0); err != nil {
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
		if err := validateBitfields(record, false); err != nil {
			return err
		}
		if err := validateAlignments(record, false); err != nil {
			return err
		}
		if err := validatePacking(record); err != nil {
			return err
		}
		if err := validateMemberAlignments(record, false); err != nil {
			return err
		}
		if err := validatePackedFields(record); err != nil {
			return err
		}
		if (record.IndirectArgument || record.IndirectResult) && (h.Target.OS != "windows" || h.Target.Arch != "386" || !strings.Contains(h.Target.Triple, "msvc")) {
			return fmt.Errorf("clang: compiler-observed indirect records require an MSVC i386 target")
		}
		for _, pointer := range record.FunctionPointers {
			if pointer.Position < 0 {
				return fmt.Errorf("clang: invalid function-pointer field position")
			}
		}
		if err := validateFunctionPointers(recordFieldSignature(record), record.FunctionPointers, &budget, 0); err != nil {
			return err
		}
	}
	return nil
}

func validateFunctionPointers(signature abi.Signature, pointers []FunctionPointer, budget *int, depth int) error {
	if depth > 8 {
		return fmt.Errorf("clang: function-pointer nesting exceeds 8 levels")
	}
	if len(pointers) > len(signature.Args)+1 {
		return fmt.Errorf("clang: too many function-pointer declarations")
	}
	seen := make(map[int]bool)
	for _, pointer := range pointers {
		if pointer.Position < -1 || pointer.Position >= len(signature.Args) || seen[pointer.Position] {
			return fmt.Errorf("clang: invalid or duplicate function-pointer position")
		}
		seen[pointer.Position] = true
		if pointer.pointerDepth() < 1 || pointer.pointerDepth() > 8 {
			return fmt.Errorf("clang: pointer indirection requires 1 to 8 levels")
		}
		description := signature.ReturnType()
		if pointer.Position >= 0 {
			description = signature.ArgumentType(pointer.Position)
		}
		if description.Type != abi.Pointer || description.Elem != nil {
			return fmt.Errorf("clang: function-pointer position must be an opaque pointer")
		}
		if err := validateSignature(pointer.Signature, budget); err != nil {
			return err
		}
		if err := validateFunctionPointers(pointer.Signature, pointer.FunctionPointers, budget, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (h *Header) validateHostFunctionPointers(pointers []FunctionPointer) error {
	for _, pointer := range pointers {
		if err := h.validateHostRecords(pointer.Signature); err != nil {
			return err
		}
		if err := h.validateHostFunctionPointers(pointer.FunctionPointers); err != nil {
			return err
		}
	}
	return nil
}

func validateSignature(s abi.Signature, budget *int) error {
	if len(s.Args) > 256 {
		return fmt.Errorf("clang: at most 256 declared parameters are supported")
	}
	for _, description := range s.ArgTypes {
		if err := consumeDescription(description, budget, 0); err != nil {
			return err
		}
	}
	if s.ResultType != nil {
		if err := consumeDescription(*s.ResultType, budget, 0); err != nil {
			return err
		}
	}
	*budget -= len(s.Args) + 1
	if *budget < 0 {
		return fmt.Errorf("clang: descriptions exceed %d nodes", maxDescriptionNodes)
	}
	return s.Validate()
}

func validRecordLayout(record Record) error {
	if err := validatePacking(record); err != nil {
		return err
	}
	if err := validatePackedFields(record); err != nil {
		return err
	}
	if err := validateBitfields(record, true); err != nil {
		return err
	}
	if err := validateAlignments(record, true); err != nil {
		return err
	}
	if err := validateMemberAlignments(record, true); err != nil {
		return err
	}
	l := record.Layout
	if l.Size == 0 || l.Size > 65536 || l.Alignment == 0 || l.Alignment > l.Size || l.Alignment&(l.Alignment-1) != 0 || l.Size%l.Alignment != 0 || len(l.Offsets) != len(record.Description.Fields) {
		return fmt.Errorf("clang: invalid compiler layout for %s", record.Name)
	}
	for i, offset := range l.Offsets {
		sharedBits := i > 0 && record.bitfieldAt(i) != nil && record.bitfieldAt(i-1) != nil
		leadingPadding := len(record.Padding) != 0 && record.Padding[0].Before == 0
		if offset >= l.Size || i == 0 && offset != 0 && !leadingPadding || i > 0 && (offset < l.Offsets[i-1] || offset == l.Offsets[i-1] && !sharedBits) {
			return fmt.Errorf("clang: invalid compiler offsets for %s", record.Name)
		}
	}
	return nil
}

func (h *Header) validateHostRecords(signature abi.Signature) error {
	var check func(abi.TypeDesc) error
	var checkSignature func(abi.Signature) error
	var checkPointers func([]FunctionPointer) error
	seen := make(map[int]bool)
	checkPointers = func(pointers []FunctionPointer) error {
		for _, pointer := range pointers {
			if err := checkSignature(pointer.Signature); err != nil {
				return err
			}
			if err := checkPointers(pointer.FunctionPointers); err != nil {
				return err
			}
		}
		return nil
	}
	checkSignature = func(s abi.Signature) error {
		if err := check(s.ReturnType()); err != nil {
			return err
		}
		for i := range s.Args {
			if err := check(s.ArgumentType(i)); err != nil {
				return err
			}
		}
		return nil
	}
	check = func(d abi.TypeDesc) error {
		// Inspect nested records before asking the backend for a parent layout:
		// packed children require a compiler adapter even without libffi installed.
		for _, field := range d.Fields {
			if err := check(field.Type); err != nil {
				return err
			}
		}
		if d.Elem != nil {
			if err := check(*d.Elem); err != nil {
				return err
			}
		}
		if d.Type == abi.Struct {
			found := false
			var native abi.Layout
			for i, record := range h.Records {
				if !reflect.DeepEqual(record.Description, d) {
					continue
				}
				if err := validRecordLayout(record); err != nil {
					return err
				}
				if record.requiresCAdapter() {
					return fmt.Errorf("clang: record %s requires a compiled C adapter", record.Name)
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
				if !seen[i] {
					seen[i] = true
					if err := checkPointers(record.FunctionPointers); err != nil {
						return err
					}
				}
			}
			if !found {
				return fmt.Errorf("clang: missing compiler record layout")
			}
		}
		return nil
	}
	return checkSignature(signature)
}

func (h *Header) recordProbes() string {
	declarations, boundaries := h.paddingProbes()
	alignmentTypes, alignmentValues := h.bitfieldAlignmentProbes()
	var source strings.Builder
	source.WriteString(declarations)
	source.WriteString(alignmentTypes)
	fmt.Fprintln(&source, "enum {")
	source.WriteString(boundaries)
	source.WriteString(alignmentValues)
	for i, record := range h.Records {
		fmt.Fprintf(&source, "dylib_record_%d_size=sizeof(%s),\ndylib_record_%d_align=_Alignof(%s),\n", i, record.Name, i, record.Name)
		for j, field := range record.Description.Fields {
			if record.bitfieldAt(j) != nil {
				continue
			}
			fmt.Fprintf(&source, "dylib_record_%d_offset_%d=__builtin_offsetof(%s,%s),\n", i, j, record.Name, field.Name)
			if record.MaxFieldAlignment != 0 {
				fmt.Fprintf(&source, "dylib_record_%d_memberalign_%d=__alignof__(((%s*)0)->%s),\n", i, j, record.Name, field.Name)
			}
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
			if h.Records[i].bitfieldAt(j) != nil {
				layout.Offsets = append(layout.Offsets, 0)
				continue
			}
			offset, err := get(fmt.Sprintf("dylib_record_%d_offset_%d", i, j))
			if err != nil {
				return err
			}
			layout.Offsets = append(layout.Offsets, offset)
		}
		h.Records[i].Layout = layout
		r := &h.Records[i]
		members := recordMembers(*r)
		starts := bitfieldUnitStarts(*r, members)
		if r.MaxFieldAlignment != 0 {
			namedStarts := make(map[int]bool)
			for position := range starts {
				member := members[position]
				if member.field >= 0 {
					namedStarts[member.field] = true
				}
			}
			r.MemberAlignments = nil
			for j, offset := range layout.Offsets {
				if r.bitfieldAt(j) != nil && (!h.Target.msLayout() || !namedStarts[j]) {
					r.MemberAlignments = append(r.MemberAlignments, 0)
					continue
				}
				alignment, err := get(fmt.Sprintf("dylib_record_%d_memberalign_%d", i, j))
				if err != nil {
					return err
				}
				if r.bitfieldAt(j) == nil {
					alignment, err = memberStorageAlignment(alignment, layout.Alignment, offset)
				}
				if err != nil {
					return fmt.Errorf("clang: %s: %w", h.Records[i].Name, err)
				}
				h.Records[i].MemberAlignments = append(h.Records[i].MemberAlignments, alignment)
			}
		}
		for position, member := range members {
			if member.padding >= 0 && r.MaxFieldAlignment != 0 && h.Target.msLayout() && starts[position] {
				r.Padding[member.padding].StorageAlignment, err = get(fmt.Sprintf("dylib_record_%d_paddingalign_%d", i, member.padding))
				if err != nil {
					return err
				}
			}
		}
		for j := range h.Records[i].Padding {
			p := &h.Records[i].Padding[j]
			if p.Width == 0 {
				p.Offset, err = get(fmt.Sprintf("dylib_record_%d_padding_%d", i, j))
				if err != nil {
					return err
				}
			}
		}
		if len(h.Records[i].Bitfields) == 0 {
			if err := validRecordLayout(h.Records[i]); err != nil {
				return err
			}
		}
	}
	return nil
}
