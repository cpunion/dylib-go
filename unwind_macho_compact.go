package dylib

import (
	"encoding/binary"
	"fmt"
)

type machoUnwindRange struct {
	start, end, dwarf, size, compact, compactSize uintptr
	personality                                   bool
}

func (im *image) machOCompactCount() (uint64, error) {
	var count uint64
	for _, o := range im.objects {
		if o.info.Format != "Mach-O" {
			continue
		}
		for _, s := range o.sections {
			if s == nil || s.name != "__compact_unwind" {
				continue
			}
			if im.pointerSize != 8 || s.exec || s.write || s.size%32 != 0 || uint64(len(s.data)) != s.size {
				return 0, fmt.Errorf("invalid Mach-O compact unwind record storage")
			}
			count += s.size / 32
			if count > 65536 {
				return 0, fmt.Errorf("too many Mach-O compact unwind records")
			}
		}
	}
	return count, nil
}

func (im *image) machOCompactRanges(dwarf []machoUnwindRange) ([]machoUnwindRange, error) {
	if !im.machoHeader {
		return dwarf, nil
	}
	le := binary.LittleEndian
	le.PutUint32(im.mem, 0xfeedfacf) // MH_MAGIC_64
	switch im.objects[0].info.Arch {
	case "amd64":
		le.PutUint32(im.mem[4:], 0x01000007)
		le.PutUint32(im.mem[8:], 3) // CPU_SUBTYPE_X86_64_ALL
	case "arm64":
		le.PutUint32(im.mem[4:], 0x0100000c)
	default:
		return nil, fmt.Errorf("unsupported Mach-O compact unwind architecture")
	}
	le.PutUint32(im.mem[12:], 1) // MH_OBJECT; no load commands.
	selected := make(map[uintptr]machoUnwindRange)
	for _, r := range dwarf {
		if _, exists := selected[r.start]; exists {
			return nil, fmt.Errorf("overlapping dynamic Mach-O frame ranges")
		}
		selected[r.start] = r
	}
	var used uint64
	compactStarts := make(map[uint64]bool)
	for _, o := range im.objects {
		losingCode := make(map[uint64]bool)
		for index, symbol := range o.symbols {
			if !symbol.global || !symbol.weak || symbol.section <= 0 || symbol.section >= len(o.sections) {
				continue
			}
			code := o.sections[symbol.section]
			if code == nil || !code.exec {
				continue
			}
			definition, err := im.resolveDefinition(o, index)
			if err != nil {
				return nil, err
			}
			if definition.o != o || definition.index != index {
				losingCode[uint64(im.base)+code.offset+symbol.value] = true
			}
		}
		for index, s := range o.sections {
			if s == nil || s.name != "__compact_unwind" {
				continue
			}
			losing := make(map[uint64]bool)
			for _, relocation := range o.relocs {
				if relocation.section != index || relocation.offset%32 != 0 || relocation.local {
					continue
				}
				if relocation.symbol < 0 || relocation.symbol >= len(o.symbols) {
					return nil, fmt.Errorf("invalid compact unwind function symbol")
				}
				symbol := o.symbols[relocation.symbol]
				if symbol.global && symbol.weak {
					definition, err := im.resolveDefinition(o, relocation.symbol)
					if err != nil {
						return nil, err
					}
					losing[relocation.offset] = definition.o != o || definition.index != relocation.symbol
				}
			}
			for offset := uint64(0); offset < s.size; offset += 32 {
				if losing[offset] {
					continue
				}
				data := im.mem[s.offset+offset : s.offset+offset+32]
				start := le.Uint64(data)
				if losingCode[start] {
					continue
				}
				length := uint64(le.Uint32(data[8:]))
				encoding := le.Uint32(data[12:])
				personality, lsda := le.Uint64(data[16:]), le.Uint64(data[24:])
				owned := false
				for _, code := range o.sections {
					if code != nil && code.exec && start >= uint64(im.base)+code.offset && start-uint64(im.base)-code.offset < code.size && length <= code.size-(start-uint64(im.base)-code.offset) {
						owned = true
						break
					}
				}
				if !owned || length == 0 || encoding&0x30000000 != 0 {
					return nil, fmt.Errorf("invalid compact unwind function range/personality index")
				}
				if lsda != 0 && !im.frameLSDA(lsda) || lsda == 0 && encoding&0x40000000 != 0 {
					return nil, fmt.Errorf("compact unwind LSDA is outside read-only exception data")
				}
				mode := encoding & 0x0f000000
				r, hasDwarf := selected[uintptr(start)]
				hasDwarf = hasDwarf && r.dwarf != 0
				if mode == 0 {
					if personality != 0 || lsda != 0 {
						return nil, fmt.Errorf("language data without compact unwind encoding")
					}
					continue // An explicit no-unwind record supplies no frame.
				}
				if compactStarts[start] {
					return nil, fmt.Errorf("duplicate compact unwind function range")
				}
				compactStarts[start] = true
				dwarfMode := uint32(0x04000000)
				if o.info.Arch == "arm64" {
					dwarfMode = 0x03000000
				}
				if mode == dwarfMode {
					if !hasDwarf {
						return nil, fmt.Errorf("%s: compact DWARF mode has no retained FDE at image offset %#x", o.info.Name, start-uint64(im.base))
					}
					continue // The finder can return the original DWARF section directly.
				}
				if err := validateCompactEncoding(o.info.Arch, encoding); err != nil {
					return nil, err
				}
				if o.info.Arch == "amd64" && mode == 0x03000000 {
					instruction := uint64(encoding>>16) & 255
					if instruction < 3 || instruction+4 > length {
						return nil, fmt.Errorf("compact stack-size instruction exceeds its function")
					}
					codeOffset := start - uint64(im.base) + instruction
					code := im.mem[codeOffset-3 : codeOffset+4]
					allocation := uint64(le.Uint32(code[3:]))
					stack := allocation + uint64(encoding>>13&7)*8
					if code[0] != 0x48 || code[1] != 0x81 || code[2] != 0xec || allocation > 0x7fffffff || stack < uint64((encoding>>10&7)+1)*8 {
						return nil, fmt.Errorf("invalid compact indirect stack-size instruction")
					}
				}
				if hasDwarf && personality == 0 {
					continue // Keep FDE lookup for C frames; native language records use compact.
				}
				if used+80 > im.compactSize {
					return nil, fmt.Errorf("compact unwind tables exceed reserved storage")
				}
				tableOffset := im.compactStart + used
				used += 80
				table := im.mem[tableOffset : tableOffset+80]
				function := uint32(start - uint64(im.base))
				end := uint32(start + length - uint64(im.base))
				le.PutUint32(table, 1) // unwind_info_section_header, version 1.
				le.PutUint32(table[4:], 28)
				le.PutUint32(table[12:], 28)
				if personality != 0 {
					le.PutUint32(table[16:], 1)
					le.PutUint32(table[28:], uint32(s.offset+offset+16)) // RVA of the native pointer cell.
					encoding |= 0x10000000
				}
				le.PutUint32(table[20:], 32)
				le.PutUint32(table[24:], 2) // One range and its end sentinel.
				le.PutUint32(table[32:], function)
				le.PutUint32(table[36:], 56)
				le.PutUint32(table[40:], 72)
				le.PutUint32(table[44:], end)
				le.PutUint32(table[52:], 72)
				le.PutUint32(table[56:], 2) // UNWIND_SECOND_LEVEL_REGULAR.
				le.PutUint16(table[60:], 8)
				le.PutUint16(table[62:], 1)
				le.PutUint32(table[64:], function)
				if lsda != 0 {
					encoding |= 0x40000000
					le.PutUint32(table[52:], 80)
					le.PutUint32(table[72:], function)
					le.PutUint32(table[76:], uint32(lsda-uint64(im.base)))
				}
				le.PutUint32(table[68:], encoding)
				selected[uintptr(start)] = machoUnwindRange{start: uintptr(start), end: uintptr(start + length), compact: im.base + uintptr(tableOffset), compactSize: 80}
			}
		}
	}
	im.compactUsed = used
	result := make([]machoUnwindRange, 0, len(selected))
	for _, r := range selected {
		result = append(result, r)
	}
	return result, nil
}

func validateCompactEncoding(arch string, encoding uint32) error {
	mode, payload := encoding&0x0f000000, encoding&0x00ffffff
	if arch == "arm64" {
		if mode == 0x04000000 && payload&^uint32(0xf1f) == 0 || mode == 0x02000000 && payload&^uint32(0xfff000) == 0 {
			return nil
		}
	} else if arch == "amd64" && mode == 0x01000000 && payload&^uint32(0xff7fff) == 0 {
		seen := uint32(0)
		for i := 0; i < 5; i++ {
			register := payload >> (3 * i) & 7
			if register >= 6 || register != 0 && (seen&(1<<register) != 0 || uint32(i) >= payload>>16) {
				return fmt.Errorf("invalid compact RBP saved registers")
			}
			seen |= 1 << register
		}
		return nil
	}
	if arch == "amd64" && (mode == 0x02000000 || mode == 0x03000000) {
		count := payload >> 10 & 7
		if count > 6 {
			return fmt.Errorf("invalid compact frameless register count")
		}
		permutations := uint32(1)
		for i := uint32(0); i < count; i++ {
			permutations *= 6 - i
		}
		if payload&0x3ff >= permutations {
			return fmt.Errorf("invalid compact saved-register permutation")
		}
		if mode == 0x02000000 && (payload>>16 < count+1 || payload&0xe000 != 0) {
			return fmt.Errorf("invalid compact immediate stack size")
		}
		return nil
	}
	return fmt.Errorf("unsupported compact unwind encoding %#x for %s", encoding, arch)
}
