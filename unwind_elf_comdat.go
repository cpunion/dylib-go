package dylib

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// A shared .eh_frame can describe both retained code and discarded COMDAT
// copies. Remove the losing FDEs before dependency discovery and relocation,
// preserving input bytes and repairing CIE back-references and relocation offsets.
func (o *object) pruneELFFrames(discarded map[int]bool) error {
	if o.info.Format != "ELF" || len(discarded) == 0 {
		return nil
	}
	for sectionIndex, section := range o.sections {
		if section == nil || !elfUnwindSection(section.name) {
			continue
		}
		type record struct {
			start, end, next int
			id               uint32
			drop             bool
		}
		var records []record
		discardedPC := make(map[uint64]bool)
		for _, relocation := range o.relocs {
			if relocation.section == sectionIndex && relocation.symbol >= 0 && relocation.symbol < len(o.symbols) && discarded[o.symbols[relocation.symbol].section] {
				discardedPC[relocation.offset] = true
			}
		}
		changed := false
		for offset := 0; offset < len(section.data); {
			if len(section.data)-offset < 4 {
				return fmt.Errorf("truncated COMDAT ELF frame length")
			}
			length := binary.LittleEndian.Uint32(section.data[offset:])
			if length == 0 {
				if !allZero(section.data[offset:]) {
					return fmt.Errorf("data after COMDAT frame terminator")
				}
				break
			}
			if length == 0xffffffff || length < 4 || uint64(length) > uint64(len(section.data)-offset-4) {
				return fmt.Errorf("invalid COMDAT ELF frame record")
			}
			r := record{start: offset, end: offset + 4 + int(length), id: binary.LittleEndian.Uint32(section.data[offset+4:])}
			if r.id != 0 {
				r.drop = discardedPC[uint64(offset+8)]
				changed = changed || r.drop
			}
			records = append(records, r)
			offset = r.end
		}
		if !changed {
			continue
		}
		// Non-frame references to section-relative offsets would need their own
		// relocation/addend rebasing. Compiler .eh_frame_hdr is already excluded.
		for _, relocation := range o.relocs {
			if relocation.section != sectionIndex && !discarded[relocation.section] && relocation.symbol >= 0 && relocation.symbol < len(o.symbols) && o.symbols[relocation.symbol].section == sectionIndex {
				return fmt.Errorf("cannot prune externally referenced ELF frame section")
			}
		}
		data := make([]byte, 0, len(section.data))
		cies := make(map[int]int)
		for index := range records {
			r := &records[index]
			if r.drop {
				continue
			}
			r.next = len(data)
			data = append(data, section.data[r.start:r.end]...)
			if r.id == 0 {
				cies[r.start] = r.next
			} else {
				target := int64(r.start) + 4 - int64(r.id)
				cie, ok := cies[int(target)]
				if target < 0 || target >= int64(r.start) || !ok {
					return fmt.Errorf("invalid COMDAT FDE CIE reference")
				}
				binary.LittleEndian.PutUint32(data[r.next+4:], uint32(r.next+4-cie))
			}
		}
		remap := func(offset uint64) (uint64, bool) {
			index := sort.Search(len(records), func(i int) bool { return uint64(records[i].end) > offset })
			if index < len(records) {
				r := records[index]
				return uint64(r.next) + offset - uint64(r.start), !r.drop
			}
			return 0, false
		}
		var relocs []relocation
		for _, relocation := range o.relocs {
			if relocation.section == sectionIndex {
				offset, keep := remap(relocation.offset)
				if !keep {
					continue
				}
				relocation.offset = offset
			}
			relocs = append(relocs, relocation)
		}
		o.relocs = relocs
		for index := range o.symbols {
			symbol := &o.symbols[index]
			if symbol.section != sectionIndex {
				continue
			}
			if symbol.size != 0 {
				return fmt.Errorf("cannot prune sized ELF frame symbols")
			}
			value, keep := remap(symbol.value)
			if keep {
				symbol.value = value
			} else {
				symbol.section, symbol.value = -3, 0
			}
		}
		section.data, section.size = data, uint64(len(data))
	}
	return nil
}
