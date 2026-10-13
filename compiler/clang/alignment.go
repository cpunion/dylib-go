package clang

import "fmt"

// FieldAlignment preserves an explicit alignment on a named C field. Position
// indexes the logical fields, including named bitfields and fixed arrays.
type FieldAlignment struct {
	Position int
	Aligned  uint64
}

func validAlignment(value uint64) bool {
	return value <= 65536 && (value == 0 || value&(value-1) == 0)
}

func explicitAlignment(node astNode) (uint64, error) {
	value, ok := constantValue(node)
	if !ok || value < 0 || !validAlignment(uint64(value)) {
		return 0, fmt.Errorf("alignment requires an evaluated power of two at most 65536")
	}
	return uint64(value), nil
}

// An omitted GNU aligned argument is represented by one empty AST child.
// Type-based _Alignas has no child; its effective member alignment needs a
// compiler probe because Clang's JSON does not retain the requested type.
func omittedAlignment(node astNode) bool {
	return len(node.Inner) == 1 && node.Inner[0].Kind == ""
}

func (r *typeResolver) recordAlignment(node astNode) (uint64, error) {
	if omittedAlignment(node) {
		value := r.sizes["max_alignment"]
		if value <= 0 || !validAlignment(uint64(value)) {
			return 0, fmt.Errorf("invalid compiler maximum alignment")
		}
		return uint64(value), nil
	}
	return explicitAlignment(node)
}

func (r *typeResolver) memberAlignment(record string, field, node astNode) (uint64, error) {
	if _, ok := constantValue(node); ok || omittedAlignment(node) {
		return r.recordAlignment(node)
	}
	if len(node.Inner) != 0 || field.IsBitfield {
		return explicitAlignment(node)
	}
	if r.fieldProbes == nil {
		return 0, errRecordFieldTypes
	}
	probe, ok := r.fieldProbes[recordField{record, field.Name}]
	if !ok || probe.Alignment == 0 || !validAlignment(probe.Alignment) {
		return 0, fmt.Errorf("missing compiler member alignment")
	}
	return probe.Alignment, nil
}

func (r Record) fieldAlignment(position int) uint64 {
	for _, field := range r.FieldAlignments {
		if field.Position == position {
			return field.Aligned
		}
	}
	return 0
}

func validateAlignments(r Record, complete bool) error {
	if !validAlignment(r.Aligned) {
		return fmt.Errorf("clang: invalid record alignment in %s", r.Name)
	}
	seen := make(map[int]bool)
	for _, field := range r.FieldAlignments {
		if field.Position < 0 || field.Position >= len(r.Description.Fields) || seen[field.Position] || field.Aligned == 0 || !validAlignment(field.Aligned) {
			return fmt.Errorf("clang: invalid or duplicate field alignment in %s", r.Name)
		}
		seen[field.Position] = true
		alignment := field.Aligned
		// GNU packing can cap a source request; MSVC can preserve it. The
		// separately probed storage alignments validate the actual placement.
		if r.MaxFieldAlignment != 0 && !r.hasBitfields() && alignment > r.MaxFieldAlignment {
			alignment = r.MaxFieldAlignment
		}
		if complete && (len(r.Layout.Offsets) != len(r.Description.Fields) || r.Layout.Alignment < alignment || r.Layout.Offsets[field.Position]%alignment != 0) {
			return fmt.Errorf("clang: compiler field alignment mismatch in %s", r.Name)
		}
	}
	if complete && r.Layout.Alignment < r.Aligned {
		return fmt.Errorf("clang: compiler record alignment mismatch in %s", r.Name)
	}
	return nil
}

// Only native declarations receive these attributes; transport records retain
// ordinary fields that both Go compilers can represent.
func (r Record) cAttributes() string {
	if r.Packed && r.Aligned != 0 {
		return fmt.Sprintf("__attribute__((packed,aligned(%d))) ", r.Aligned)
	}
	if r.Packed {
		return "__attribute__((packed)) "
	}
	if r.Aligned != 0 {
		return fmt.Sprintf("__attribute__((aligned(%d))) ", r.Aligned)
	}
	return ""
}

func (r Record) cFieldAttributes(position int) string {
	packing := ""
	if r.fieldPacked(position) {
		packing = " __attribute__((packed))"
	}
	if alignment := r.fieldAlignment(position); alignment != 0 {
		return packing + fmt.Sprintf(" __attribute__((aligned(%d)))", alignment)
	}
	return packing
}
