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
		if complete && (len(r.Layout.Offsets) != len(r.Description.Fields) || r.Layout.Alignment < field.Aligned || r.Layout.Offsets[field.Position]%field.Aligned != 0) {
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
