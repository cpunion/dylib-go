package dylib

import (
	"debug/elf"
	"fmt"
	"math"
)

const (
	elfSize32    = uint32(elf.R_X86_64_SIZE32)
	elfSize64    = uint32(elf.R_X86_64_SIZE64)
	elf386Size32 = uint32(elf.R_386_SIZE32)
)

func elfSizeRelocation(o *object, r relocation) bool {
	return o.info.Arch == "amd64" && (r.typ == elfSize32 || r.typ == elfSize64) || o.info.Arch == "386" && r.typ == elf386Size32
}

// SIZE relocations use the selected definition's size, not its address. Native
// address-only providers cannot supply this metadata; even a weak reference to
// a resolved external symbol needs a definition with a known ELF size.
func (im *image) elfSymbolSize(o *object, index int) (uint64, error) {
	d, err := im.resolveDefinition(o, index)
	if err != nil {
		return 0, err
	}
	o, s := d.o, d.sym()
	switch s.section {
	case 0:
		if s.name == "" && d.index == 0 || s.weak && im.externalSymbol(o, s.name) == 0 {
			return 0, nil
		}
		return 0, fmt.Errorf("ELF symbol size unavailable for %s: requires an object definition", s.name)
	case -2:
		allocation, ok := im.common[s.name]
		if !ok {
			return 0, fmt.Errorf("unallocated common %s", s.name)
		}
		return allocation.size, nil
	case -1:
		return s.size, nil
	default:
		if s.section < 0 || s.section >= len(o.sections) || o.sections[s.section] == nil || s.value > o.sections[s.section].size {
			return 0, fmt.Errorf("symbol %s refers outside a loaded object section", s.name)
		}
		return s.size, nil
	}
}

func (im *image) relocELFSize(o *object, r relocation, b []byte) error {
	size, err := im.elfSymbolSize(o, r.symbol)
	if err != nil {
		return err
	}
	if o.info.Arch == "386" {
		value := uint32(size) + uint32(r.addend)
		if r.implicit {
			value += le.Uint32(b)
		}
		le.PutUint32(b, value) // i386 uses modulo-2^32 arithmetic.
		return nil
	}
	if r.typ == elfSize64 {
		le.PutUint64(b, size+uint64(r.addend))
		return nil
	}
	// Match GNU ld's unsigned SIZE32 range check without converting a possibly
	// large st_size to int64 or overflowing while applying a signed addend.
	value := size
	if r.addend < 0 {
		delta := uint64(-(r.addend + 1)) + 1
		if delta > value {
			return fmt.Errorf("unsigned 32-bit ELF size relocation overflow")
		}
		value -= delta
	} else {
		if value > math.MaxUint32 || uint64(r.addend) > math.MaxUint32-value {
			return fmt.Errorf("unsigned 32-bit ELF size relocation overflow")
		}
		value += uint64(r.addend)
	}
	if value > math.MaxUint32 {
		return fmt.Errorf("unsigned 32-bit ELF size relocation overflow")
	}
	le.PutUint32(b, uint32(value))
	return nil
}
