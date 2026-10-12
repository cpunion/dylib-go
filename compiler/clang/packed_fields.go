package clang

import "fmt"

func (r Record) fieldPacked(position int) bool {
	for _, field := range r.PackedFields {
		if field == position {
			return true
		}
	}
	return false
}

func validatePackedFields(record Record) error {
	previous := -1
	for _, position := range record.PackedFields {
		if position <= previous || position >= len(record.Description.Fields) || record.bitfieldAt(position) != nil {
			return fmt.Errorf("clang: invalid packed fields for %s", record.Name)
		}
		previous = position
	}
	return nil
}
