package dylib

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

type startupPart struct {
	object  *object
	section *section
	index   int
}
type startupGroup struct {
	name, arch   string
	offset, size uint64
	parts        []startupPart
}

// Group executable fragments in original input order, not lazy archive
// discovery order. Each complete group owns whole executable pages.
func (im *image) layoutStartup(size uint64) (uint64, error) {
	for _, name := range []string{".init", ".fini"} {
		group := startupGroup{name: name}
		for _, o := range im.objects {
			for index, section := range o.sections {
				if section != nil && section.startup && section.name == name && section.size != 0 {
					if o.info.Format != "ELF" || !section.exec || section.write || section.tail != 0 || uint64(len(section.data)) != section.size || section.align > im.page {
						return 0, fmt.Errorf("%s: invalid executable ELF startup fragment", name)
					}
					if group.arch != "" && group.arch != o.info.Arch {
						return 0, fmt.Errorf("%s: mixed startup architectures", name)
					}
					group.arch = o.info.Arch
					if group.arch == "arm64" && (section.size%4 != 0 || section.align != 0 && section.align < 4) {
						return 0, fmt.Errorf("%s: unaligned AArch64 startup fragment", name)
					}
					group.parts = append(group.parts, startupPart{o, section, index})
				}
			}
		}
		if len(group.parts) == 0 {
			continue
		}
		sort.SliceStable(group.parts, func(i, j int) bool {
			a, b := group.parts[i], group.parts[j]
			if a.object.inputOrder != b.object.inputOrder {
				return a.object.inputOrder < b.object.inputOrder
			}
			if a.object.memberOrder != b.object.memberOrder {
				return a.object.memberOrder < b.object.memberOrder
			}
			return a.index < b.index
		})
		first, last := group.parts[0], group.parts[len(group.parts)-1]
		entry := name[1:]
		entry = "_" + entry
		def, ok := im.defs[entry]
		if !ok || def.o != first.object || def.sym().section != first.index || def.sym().value != 0 {
			return 0, fmt.Errorf("%s: startup entry %s must begin the first fragment", name, entry)
		}
		if !standardStartupFrame(group.arch, first.section.data, last.section.data) {
			return 0, fmt.Errorf("%s: incomplete or unsupported ELF CRT prologue/epilogue", name)
		}
		group.offset = alignUp(size, im.page)
		cursor := group.offset
		for _, part := range group.parts {
			alignment := part.section.align
			if group.arch == "arm64" && alignment < 4 {
				alignment = 4
			}
			cursor = alignUp(cursor, alignment)
			part.section.offset = cursor
			cursor += part.section.size
			if cursor > maxImage {
				return 0, fmt.Errorf("ELF startup group exceeds image limit")
			}
		}
		size = alignUp(cursor, im.page)
		group.size = size - group.offset
		im.startup = append(im.startup, group)
	}
	return size, nil
}

// Validate the standard split CRT frame before accepting fall-through code.
// This is a conservative completeness contract, not arbitrary instruction
// verification. User fragments are trusted native code like other objects.
func standardStartupFrame(arch string, begin, end []byte) bool {
	switch arch {
	case "amd64":
		begin = bytes.TrimPrefix(begin, []byte{0xf3, 0x0f, 0x1e, 0xfa}) // endbr64
		return bytes.HasPrefix(begin, []byte{0x48, 0x83, 0xec, 8}) && bytes.HasSuffix(end, []byte{0x48, 0x83, 0xc4, 8, 0xc3})
	case "386":
		begin = bytes.TrimPrefix(begin, []byte{0xf3, 0x0f, 0x1e, 0xfb}) // endbr32
		return bytes.HasPrefix(begin, []byte{0x53, 0x83, 0xec, 8}) && bytes.HasSuffix(end, []byte{0x83, 0xc4, 8, 0x5b, 0xc3})
	case "arm64":
		// Older glibc leaves a NOP where newer builds sign the return address;
		// a BTI landing pad may precede either frame form.
		begin = bytes.TrimPrefix(begin, []byte{0x5f, 0x24, 0x03, 0xd5}) // bti c
		begin = bytes.TrimPrefix(begin, []byte{0x1f, 0x20, 0x03, 0xd5}) // nop
		pac := []byte{0x3f, 0x23, 0x03, 0xd5}                           // paciasp
		signed := bytes.HasPrefix(begin, pac)
		begin = bytes.TrimPrefix(begin, pac)
		suffix := []byte{0xfd, 0x7b, 0xc1, 0xa8, 0xc0, 0x03, 0x5f, 0xd6} // ldp x29,x30,[sp],16; ret
		if signed {
			suffix = []byte{0xfd, 0x7b, 0xc1, 0xa8, 0xbf, 0x23, 0x03, 0xd5, 0xc0, 0x03, 0x5f, 0xd6}
		}
		return bytes.HasPrefix(begin, []byte{0xfd, 0x7b, 0xbf, 0xa9}) && bytes.HasSuffix(end, suffix)
	}
	return false
}

func (im *image) fillStartupPadding() error {
	for _, group := range im.startup {
		region := im.mem[group.offset : group.offset+group.size]
		switch group.arch {
		case "amd64", "386":
			for i := range region {
				region[i] = 0x90
			}
		case "arm64":
			for i := 0; i < len(region); i += 4 {
				binary.LittleEndian.PutUint32(region[i:], 0xd503201f)
			}
		default:
			return fmt.Errorf("unsupported ELF startup architecture %s", group.arch)
		}
	}
	return nil
}

func (im *image) startupAddress(name string) uintptr {
	for _, group := range im.startup {
		if group.name == name {
			return im.base + uintptr(group.parts[0].section.offset)
		}
	}
	return 0
}
