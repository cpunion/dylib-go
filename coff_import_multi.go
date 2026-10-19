package dylib

import (
	"bytes"
	"sort"
	"strings"
)

type coffImportSite struct {
	section int
	offset  uint64
}

type coffImportPart struct {
	public  string
	symbols []symbol
	start   uint64
	text    *section
	relocs  []relocation
	hasText bool
}

// Partition contiguous IAT/ILT slots into bounded single-import views. Reuse
// the existing decoder for names, ordinals, descriptors and exact thunk bytes.
// No original bytes or symbol indices are changed; mixed members retain their
// ordinary sections through convertMixed after all imports are validated.
func (g coffImportGraph) convertMultiple(o *object, indexes map[string]int) (*object, bool) {
	iat, ilt, text, hint, force := indexes[".idata$5"], indexes[".idata$4"], indexes[".text"], indexes[".idata$6"], indexes[".idata$7"]
	width := uint64(o.info.Bits / 8)
	iatSection := o.sections[iat]
	count := iatSection.size / width
	if count < 2 || count > uint64(len(o.symbols)) || iatSection.size%width != 0 || uint64(len(iatSection.data)) != iatSection.size || force == 0 {
		return nil, false
	}
	forceSection := o.sections[force]
	if uint64(len(forceSection.data)) != forceSection.size || (forceSection.size != 4 && forceSection.size != 4*count) {
		return nil, false
	}
	if ilt != 0 && (o.sections[ilt].size != iatSection.size || !bytes.Equal(o.sections[ilt].data, iatSection.data)) {
		return nil, false
	}
	if text != 0 && uint64(len(o.sections[text].data)) != o.sections[text].size {
		return nil, false
	}
	parts := make([]coffImportPart, int(count))
	names := make(map[string]bool)
	for _, s := range o.symbols {
		if !s.global {
			continue
		}
		if s.alias != nil || s.section == -1 || s.section == -2 {
			return nil, false
		}
		if s.section <= 0 {
			continue
		}
		if names[s.name] {
			return nil, false
		}
		names[s.name] = true
		switch s.section {
		case iat:
			if s.value%width != 0 || s.value >= iatSection.size {
				return nil, false
			}
			p := &parts[s.value/width]
			if strings.HasPrefix(s.name, "__imp_") {
				if p.public != "" {
					return nil, false
				}
				p.public = strings.TrimPrefix(s.name, "__imp_")
				if p.public == "" {
					return nil, false
				}
			}
			s.section, s.value = 3, 0
			p.symbols = append(p.symbols, s)
		case text:
		default:
			return nil, false
		}
	}
	public := make(map[string]int)
	for i, p := range parts {
		if p.public == "" {
			return nil, false
		}
		public[p.public] = i
	}
	var textRows []int
	for _, s := range o.symbols {
		if !s.global || s.section <= 0 || s.section != text {
			continue
		}
		row, ok := public[s.name]
		if !ok || parts[row].hasText || s.value > o.sections[text].size {
			return nil, false
		}
		p := &parts[row]
		p.hasText, p.start = true, s.value
		s.section, s.value = 1, 0
		p.symbols = append(p.symbols, s)
		textRows = append(textRows, row)
	}
	sort.Slice(textRows, func(i, j int) bool { return parts[textRows[i]].start < parts[textRows[j]].start })
	if text != 0 && o.sections[text].size != 0 && (len(textRows) == 0 || parts[textRows[0]].start != 0) {
		return nil, false
	}
	for i, row := range textRows {
		p := &parts[row]
		end := o.sections[text].size
		if i+1 < len(textRows) {
			end = parts[textRows[i+1]].start
		}
		if end <= p.start && o.sections[text].size != 0 {
			return nil, false
		}
		p.text = coffImportSection(o.sections[text], p.start, end-p.start)
	}
	// Index once so a large member does not scan its complete relocation array
	// for each slot. Reject duplicate sites before constructing private views.
	sites := make(map[coffImportSite]relocation)
	for _, r := range o.relocs {
		if r.section <= 0 || r.section >= len(o.sections) || o.sections[r.section] == nil || r.symbol < 0 || r.symbol >= len(o.symbols) {
			return nil, false
		}
		key := coffImportSite{r.section, r.offset}
		if _, duplicate := sites[key]; duplicate {
			return nil, false
		}
		sites[key] = r
		if r.section == text {
			index := sort.Search(len(textRows), func(i int) bool { return parts[textRows[i]].start > r.offset }) - 1
			if index < 0 || r.offset >= o.sections[text].size {
				return nil, false
			}
			row := textRows[index]
			s := o.symbols[r.symbol]
			if s.section != iat || s.value != uint64(row)*width {
				return nil, false
			}
			r.section, r.offset, r.symbol = 1, r.offset-parts[row].start, 0
			parts[row].relocs = append(parts[row].relocs, r)
			continue
		}
		step := width
		if r.section == force {
			step = 4
		} else if r.section != iat && r.section != ilt {
			return nil, false
		}
		if r.offset%step != 0 || r.offset >= o.sections[r.section].size {
			return nil, false
		}
	}
	g.dlls = make(map[coffLocation]string)
	out := *o
	out.sections, out.relocs, out.groups, out.symbols = nil, nil, nil, nil
	out.info.Symbols, out.info.Imports = nil, nil
	budget := maxFile
	for row, p := range parts {
		forceOffset := uint64(row) * 4
		if forceSection.size == 4 {
			forceOffset = 0
		}
		mini := object{info: o.info, sections: make([]*section, 6), symbols: append([]symbol(nil), p.symbols...), relocs: append([]relocation(nil), p.relocs...)}
		mini.sections[1] = p.text
		mini.sections[3] = coffImportSection(iatSection, uint64(row)*width, width)
		mini.sections[5] = coffImportSection(forceSection, forceOffset, 4)
		if ilt != 0 {
			mini.sections[2] = coffImportSection(o.sections[ilt], uint64(row)*width, width)
		}
		if hint != 0 {
			mini.sections[4] = o.sections[hint]
		}
		for _, column := range []struct{ original, view int }{{iat, 3}, {ilt, 2}, {force, 5}} {
			if column.original == 0 {
				continue
			}
			offset := uint64(row) * width
			if column.original == force {
				offset = forceOffset
			}
			r, ok := sites[coffImportSite{column.original, offset}]
			if !ok {
				continue // The shared decoder checks required and ordinal sites.
			}
			s := o.symbols[r.symbol]
			if column.original == force {
				if s.section != 0 {
					return nil, false
				}
			} else {
				if hint == 0 || s.section != hint {
					return nil, false
				}
				s.section = 4
			}
			r.section, r.offset, r.symbol = column.view, 0, len(mini.symbols)
			mini.symbols = append(mini.symbols, s)
			mini.relocs = append(mini.relocs, r)
		}
		converted, ok := g.convertPure(&mini)
		if !ok {
			return nil, false
		}
		entry := converted.info.Imports[0]
		// Force references do not establish a cross-DLL slot order. Combined
		// members must identify one DLL rather than guessing that association.
		if row != 0 && !strings.EqualFold(entry.DLL, out.info.Imports[0].DLL) {
			return nil, false
		}
		cost := len(entry.DLL) + len(entry.Symbol) + len(entry.Name)
		if cost > budget {
			return nil, false
		}
		budget -= cost
		out.symbols = append(out.symbols, converted.symbols...)
		out.info.Imports = append(out.info.Imports, converted.info.Imports...)
		out.info.Unsupported = converted.info.Unsupported
	}
	return &out, true
}

func coffImportSection(source *section, offset, size uint64) *section {
	copy := *source
	copy.data, copy.size = source.data[offset:offset+size], size
	return &copy
}
