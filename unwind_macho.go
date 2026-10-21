package dylib

import (
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/ramblelabs/dylib-go/internal/native"
)

func (im *image) registerMachOUnwind() error {
	var frames []uintptr
	var ranges []machoUnwindRange
	needsHeader := false
	for _, o := range im.objects {
		for sectionIndex, s := range o.sections {
			if s == nil || s.name != "__eh_frame" {
				continue
			}
			if s.exec || s.write {
				return fmt.Errorf("%s: invalid Mach-O frame storage", o.info.Name)
			}
			// ARM64 FDE relocations can name weak definitions coalesced into
			// another object. Retain that definition's FDE rather than its losing
			// copy's CFI. x86-64 implicit references still address retained code.
			losing := make(map[uintptr]bool)
			for _, r := range o.relocs {
				if r.section != sectionIndex || r.offset < 8 || r.pair < 0 {
					continue
				}
				if r.pair >= len(o.symbols) {
					return fmt.Errorf("invalid Mach-O frame symbol")
				}
				symbol := o.symbols[r.pair]
				if !symbol.global || !symbol.weak {
					continue
				}
				definition, err := im.resolveDefinition(o, r.pair)
				if err != nil {
					return err
				}
				if definition.o != o || definition.index != r.pair {
					losing[im.base+uintptr(s.offset+r.offset-8)] = true
				}
			}
			policy := framePolicy{pcRelative: true, language: true, record: func(start, length uint64, fde uintptr, personality bool) error {
				if losing[fde] {
					return nil
				}
				if len(ranges) == 65536 {
					return fmt.Errorf("too many dynamic Mach-O frame ranges")
				}
				ranges = append(ranges, machoUnwindRange{start: uintptr(start), end: uintptr(start + length), dwarf: im.base + uintptr(s.offset), size: uintptr(s.size), personality: personality})
				needsHeader = needsHeader || personality
				return nil
			}}
			fdes, err := im.validateFrameRecords(im.mem[s.offset:s.offset+s.size], s.offset, policy)
			if err != nil {
				return fmt.Errorf("%s: __eh_frame: %w", o.info.Name, err)
			}
			for _, fde := range fdes {
				if !losing[fde] {
					frames = append(frames, fde)
				}
			}
		}
	}
	var compactErr error
	ranges, compactErr = im.machOCompactRanges(ranges)
	if compactErr != nil {
		return compactErr
	}
	if im.machoHeader {
		if err := native.Protect(im.mem[:im.page], false, false); err != nil {
			return err
		}
		if err := native.Protect(im.mem[im.compactStart:im.compactStart+im.compactSize], false, false); err != nil {
			return err
		}
	}
	if len(ranges) == 0 {
		return nil
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	var entries []uintptr
	for index, r := range ranges {
		if index != 0 && r.start < ranges[index-1].end {
			return fmt.Errorf("overlapping dynamic Mach-O frame ranges")
		}
		entries = append(entries, r.start, r.end, r.dwarf, r.size, r.compact, r.compactSize)
		needsHeader = needsHeader || r.compact != 0
	}
	if err := im.registerDwarfFrames("/usr/lib/system/libunwind.dylib", nil); err != nil {
		return err
	}
	u := im.unwind
	add := native.Lookup(u.provider, "__unw_add_find_dynamic_unwind_sections")
	remove := native.Lookup(u.provider, "__unw_remove_find_dynamic_unwind_sections")
	if add == 0 || remove == 0 {
		if needsHeader {
			return fmt.Errorf("native language exceptions and compact frames require dynamic Mach-O unwind lookup")
		}
		register := native.Lookup(u.provider, "__register_frame")
		for _, frame := range frames {
			native.CallPointer(register, frame)
			u.frames = append(u.frames, frame)
		}
		return nil // Preserve C frame lookup on older providers without this SPI.
	}
	var err error
	header := uintptr(0)
	if im.machoHeader {
		header = im.base
	}
	u.dynamic, err = native.RegisterMachOUnwind(add, remove, header, entries)
	return err
}

// Mach-O assemblers may resolve FDE PC/LSDA references inside the input object,
// without emitting relocations (notably x86-64). Separate image pages change
// the distance between text and metadata; rebase only these implicit fields.
func (im *image) rebaseMachODWARF() error {
	for _, o := range im.objects {
		if o.info.Format != "Mach-O" {
			continue
		}
		for index, s := range o.sections {
			if s == nil || s.name != "__eh_frame" {
				continue
			}
			relocated := make(map[uint64]bool)
			for _, r := range o.relocs {
				if r.section == index {
					relocated[r.offset] = true
				}
			}
			fixup := func(offset uint64, encoding byte, lsda bool) error {
				if relocated[offset] {
					return nil
				}
				if encoding&0xf0 != 0x10 {
					return fmt.Errorf("implicit Mach-O frame references require direct PC-relative addressing")
				}
				reader := frameReader{b: im.mem[s.offset+offset : s.offset+s.size]}
				value, err := reader.encoded(encoding, im.pointerSize)
				if err != nil {
					return err
				}
				if lsda && value == 0 {
					return nil
				}
				original := int64(s.original+offset) + value
				var selected *section
				for _, target := range o.sections {
					if target == nil || original < 0 || uint64(original) < target.original || uint64(original)-target.original >= target.size {
						continue
					}
					if lsda && (!lsdaSection(o.info.Format, target.name) || target.exec || target.write) || !lsda && !target.exec {
						continue
					}
					if selected != nil {
						return fmt.Errorf("ambiguous implicit Mach-O frame reference")
					}
					selected = target
				}
				if selected == nil {
					return fmt.Errorf("implicit Mach-O frame reference is outside input text/LSDA")
				}
				delta := int64(selected.offset+uint64(original)-selected.original) - int64(s.offset+offset)
				width, err := frameEncodingSize(encoding, im.pointerSize)
				if err != nil {
					return err
				}
				b := im.mem[s.offset+offset : s.offset+offset+uint64(width)]
				if width == 8 {
					binary.LittleEndian.PutUint64(b, uint64(delta))
					return nil
				}
				return signed32(b, delta)
			}
			policy := framePolicy{pcRelative: true, language: true,
				fixup:     func(offset uint64, encoding byte) error { return fixup(offset, encoding, false) },
				lsdaFixup: func(offset uint64, encoding byte) error { return fixup(offset, encoding, true) },
			}
			if _, err := im.validateFrameRecords(im.mem[s.offset:s.offset+s.size], s.offset, policy); err != nil {
				return fmt.Errorf("%s: __eh_frame: %w", o.info.Name, err)
			}
		}
	}
	return nil
}
