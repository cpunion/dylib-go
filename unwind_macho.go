package dylib

import (
	"encoding/binary"
	"fmt"
)

func (im *image) registerMachOUnwind() error {
	var frames []uintptr
	for _, o := range im.objects {
		for _, s := range o.sections {
			if s == nil || s.name != "__eh_frame" {
				continue
			}
			if s.exec || s.write {
				return fmt.Errorf("%s: invalid Mach-O frame storage", o.info.Name)
			}
			fdes, err := im.validateDWARFFrames(im.mem[s.offset:s.offset+s.size], s.offset, true, nil)
			if err != nil {
				return fmt.Errorf("%s: __eh_frame: %w", o.info.Name, err)
			}
			frames = append(frames, fdes...)
		}
	}
	if len(frames) == 0 {
		return nil
	}
	return im.registerDwarfFrames("/usr/lib/system/libunwind.dylib", frames)
}

// Mach-O assemblers may resolve FDE PC references inside the input object,
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
			fixup := func(offset uint64, encoding byte) error {
				if relocated[offset] {
					return nil
				}
				reader := frameReader{b: im.mem[s.offset+offset : s.offset+s.size]}
				value, err := reader.encoded(encoding, im.pointerSize)
				if err != nil {
					return err
				}
				original := int64(s.original+offset) + value
				for _, text := range o.sections {
					if text == nil || !text.exec || original < 0 || uint64(original) < text.original || uint64(original)-text.original >= text.size {
						continue
					}
					delta := int64(text.offset+uint64(original)-text.original) - int64(s.offset+offset)
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
				return fmt.Errorf("implicit Mach-O FDE address is outside input text")
			}
			if _, err := im.validateDWARFFrames(im.mem[s.offset:s.offset+s.size], s.offset, true, fixup); err != nil {
				return fmt.Errorf("%s: __eh_frame: %w", o.info.Name, err)
			}
		}
	}
	return nil
}
