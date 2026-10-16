package dylib

import (
	"encoding/binary"
	"fmt"
)

// Decode fixed absolute/PC-relative personality and LSDA pointers. libgcc and
// Apple's FDE parser treat encoded zero LSDA as null before bases/indirection.
// Only image-owned data/GOT cells are dereferenced; the final personality may
// belong to an explicitly retained native runtime provider.
func (im *image) framePointer(r *frameReader, encoding byte, imageOffset uint64) (uint64, error) {
	value, err := r.encoded(encoding&^0x80, im.pointerSize)
	if err != nil || value == 0 {
		return 0, err
	}
	pointer := uint64(value)
	if encoding&0x70 == 0x10 {
		pointer += uint64(im.base) + imageOffset
	}
	if im.pointerSize == 4 {
		pointer = uint64(uint32(pointer))
	}
	if encoding&0x80 == 0 {
		return pointer, nil
	}
	if pointer < uint64(im.base) {
		return 0, fmt.Errorf("frame indirection precedes image")
	}
	offset := pointer - uint64(im.base)
	got := offset >= im.gotStart && offset <= im.gotNext && im.pointerSize <= im.gotNext-offset
	if offset > uint64(len(im.mem)) || im.pointerSize > uint64(len(im.mem))-offset || !got && !im.unwindRange(offset, im.pointerSize, false) {
		return 0, fmt.Errorf("frame indirection is outside image data/GOT")
	}
	if im.pointerSize == 4 {
		return uint64(binary.LittleEndian.Uint32(im.mem[offset:])), nil
	}
	return binary.LittleEndian.Uint64(im.mem[offset:]), nil
}

func (im *image) frameLSDA(pointer uint64) bool {
	if pointer < uint64(im.base) {
		return false
	}
	offset := pointer - uint64(im.base)
	for _, object := range im.objects {
		for _, section := range object.sections {
			if section != nil && lsdaSection(object.info.Format, section.name) && !section.exec && !section.write && uint64(len(section.data)) == section.size && offset >= section.offset && offset-section.offset < section.size {
				return true
			}
		}
	}
	return false
}

func lsdaSection(format, name string) bool {
	return format == "ELF" && elfLSDASection(name) || format == "Mach-O" && name == "__gcc_except_tab"
}
