package dylib

import (
	"debug/macho"
	"fmt"
)

// Synthetic fixups cannot collide with the four-bit on-disk relocation types.
const (
	machoPointerFixup  uint32 = 1 << 31
	machoStubFixup     uint32 = 1<<31 | 1
	machoIndirectLocal uint32 = 1 << 31
	machoIndirectAbs   uint32 = 1 << 30
)

type machoIndirectTable struct {
	section      int
	first, width uint32
}

func machoUsesIndirectTable(flags uint32) bool {
	switch flags & 0xff {
	case 6, 7, 8, 0x10:
		return true
	}
	return false
}

func machoIndirectTables(f *macho.File) ([]machoIndirectTable, error) {
	needed := false
	for _, sec := range f.Sections {
		needed = needed || machoUsesIndirectTable(sec.Flags)
	}
	if !needed {
		return nil, nil
	}
	var tables []machoIndirectTable
	ordinal := 0
	for _, load := range f.Loads {
		raw := load.Raw()
		if len(raw) < 8 {
			continue
		}
		var start, stride, countOffset, reserved int
		switch macho.LoadCmd(f.ByteOrder.Uint32(raw)) {
		case macho.LoadCmdSegment:
			start, stride, countOffset, reserved = 56, 68, 48, 60
		case macho.LoadCmdSegment64:
			start, stride, countOffset, reserved = 72, 80, 64, 68
		default:
			continue
		}
		if len(raw) < start {
			return nil, fmt.Errorf("truncated Mach-O segment command")
		}
		count := uint64(f.ByteOrder.Uint32(raw[countOffset:]))
		if count > uint64((len(raw)-start)/stride) {
			return nil, fmt.Errorf("truncated Mach-O indirect section headers")
		}
		for i := uint64(0); i < count; i++ {
			if ordinal >= len(f.Sections) {
				return nil, fmt.Errorf("invalid Mach-O section ordinal")
			}
			sec := f.Sections[ordinal]
			ordinal++
			if !machoUsesIndirectTable(sec.Flags) {
				continue
			}
			// debug/macho.SectionHeader omits reserved1/reserved2. Their raw
			// values are the indirect-table start and the symbol-stub width.
			h := raw[start+int(i)*stride:]
			width := uint32(8)
			if f.Magic != macho.Magic64 {
				width = 4
			}
			if sec.Flags&0xff == 8 {
				width = f.ByteOrder.Uint32(h[reserved+4:])
			}
			tables = append(tables, machoIndirectTable{ordinal, f.ByteOrder.Uint32(h[reserved:]), width})
		}
	}
	if ordinal != len(f.Sections) {
		return nil, fmt.Errorf("incomplete Mach-O section headers")
	}
	return tables, nil
}

func bindMachOIndirectTables(o *object, f *macho.File) error {
	tables, err := machoIndirectTables(f)
	if err != nil {
		return err
	}
	for _, table := range tables {
		sec := o.sections[table.section]
		if sec == nil {
			continue
		}
		stub := f.Sections[table.section-1].Flags&0xff == 8
		width := uint64(table.width)
		if width == 0 || sec.size%width != 0 {
			return fmt.Errorf("%s: invalid Mach-O indirect entry width", sec.name)
		}
		count, first := sec.size/width, uint64(table.first)
		if count == 0 {
			continue
		}
		if f.Dysymtab == nil || f.Symtab == nil || first > uint64(len(f.Dysymtab.IndirectSyms)) || count > uint64(len(f.Dysymtab.IndirectSyms))-first {
			return fmt.Errorf("%s: indirect symbol table range is invalid", sec.name)
		}
		if stub && (o.info.Arch != "amd64" || width != 6) && (o.info.Arch != "arm64" || width != 12) {
			o.unsupported(fmt.Sprintf("Mach-O %s symbol stubs of width %d", o.info.Arch, width))
			continue
		}
		original := make(map[uint64]relocation)
		var retained []relocation
		for _, r := range o.relocs {
			if r.section != table.section {
				retained = append(retained, r)
				continue
			}
			if stub {
				continue
			} // Regenerate canonical jumps, including their GOT references.
			if r.offset%width != 0 || r.offset >= sec.size || uint64(r.width) != width || r.typ != 0 || r.pcrel || r.pair >= 0 {
				return fmt.Errorf("%s: invalid relocation in indirect pointer entry", sec.name)
			}
			if _, exists := original[r.offset]; exists {
				return fmt.Errorf("%s: multiple relocations for indirect pointer", sec.name)
			}
			original[r.offset] = r
		}
		for i := uint64(0); i < count; i++ {
			index := f.Dysymtab.IndirectSyms[first+i]
			offset := i * width
			r := relocation{section: table.section, offset: offset, width: int(width), pair: -1}
			switch index {
			case machoIndirectLocal | machoIndirectAbs:
				if stub || f.Sections[table.section-1].Flags&0xff != 6 {
					return fmt.Errorf("%s: absolute indirect marker outside non-lazy pointers", sec.name)
				}
				if _, exists := original[offset]; exists {
					return fmt.Errorf("%s: relocation for absolute indirect pointer", sec.name)
				}
				continue // Retain the literal absolute address.
			case machoIndirectLocal:
				if stub || f.Sections[table.section-1].Flags&0xff != 6 {
					return fmt.Errorf("%s: local indirect marker outside non-lazy pointers", sec.name)
				}
				if old, exists := original[offset]; exists {
					retained = append(retained, old)
					continue
				}
				address := uint64(0)
				if width == 4 {
					address = uint64(f.ByteOrder.Uint32(sec.data[offset:]))
				} else {
					address = f.ByteOrder.Uint64(sec.data[offset:])
				}
				for n, target := range o.sections {
					if target != nil && address >= target.original && address-target.original < target.size {
						if r.local {
							return fmt.Errorf("%s: ambiguous local indirect pointer", sec.name)
						}
						r.symbol, r.local = n, true
					}
				}
				if !r.local {
					return fmt.Errorf("%s: local indirect pointer is outside object sections", sec.name)
				}
			default:
				// Only original nlist indices are valid, not synthetic N_INDR targets.
				if uint64(index) >= uint64(len(f.Symtab.Syms)) {
					return fmt.Errorf("%s: invalid indirect symbol index %#x", sec.name, index)
				}
				r.symbol, r.typ = int(index), machoPointerFixup
				if stub {
					r.typ = machoStubFixup
				}
			}
			retained = append(retained, r)
		}
		o.relocs = retained
	}
	return nil
}

func (im *image) relocateMachOIndirect(o *object, r relocation, b []byte, target, place uintptr) error {
	if r.typ == machoPointerFixup {
		if len(b) == 4 {
			return unsigned32(b, int64(target))
		}
		le.PutUint64(b, uint64(target))
		return nil
	}
	got, err := im.gotSlot(target)
	if err != nil {
		return err
	}
	switch o.info.Arch {
	case "amd64":
		copy(b, []byte{0xff, 0x25}) // jmp [rip + displacement]; preserves all argument registers.
		return signed32(b[2:], int64(got)-int64(place)-6)
	case "arm64":
		le.PutUint32(b, 0x90000010)     // adrp x16, GOT page
		le.PutUint32(b[4:], 0xf9400210) // ldr x16, [x16, GOT offset]
		le.PutUint32(b[8:], 0xd61f0200) // br x16; retain caller's link register.
		if err := armPage(b, got, place); err != nil {
			return err
		}
		return armPageOff(b[4:], got, 3)
	}
	return fmt.Errorf("unsupported Mach-O indirect stub architecture")
}
