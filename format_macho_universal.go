package dylib

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"fmt"
	"sort"
)

// The standard library fat reader handles 32-bit headers and Mach-O images.
// Decode the wrapper here to also accept 64-bit headers and regular ar slices;
// existing Go parsers still decode all payload sections, symbols and relocations.
func parseUniversal(name string, b []byte) (*file, error) {
	if len(b) < 8 {
		return nil, fmt.Errorf("truncated universal header")
	}
	be := binary.BigEndian
	width := 20
	if be.Uint32(b) == 0xcafebabf {
		width = 32
	}
	count := uint64(be.Uint32(b[4:]))
	if count == 0 || count > 64 || count > uint64(len(b)-8)/uint64(width) {
		return nil, fmt.Errorf("invalid universal architecture count (limit 64)")
	}
	tableEnd := uint64(8) + count*uint64(width)
	type arch struct {
		cpu, subcpu  uint32
		offset, size uint64
	}
	arches := make([]arch, int(count))
	seen := make(map[uint64]bool)
	for i := range arches {
		h := b[8+i*width : 8+(i+1)*width]
		a := arch{cpu: be.Uint32(h), subcpu: be.Uint32(h[4:]), offset: uint64(be.Uint32(h[8:])), size: uint64(be.Uint32(h[12:]))}
		align := be.Uint32(h[16:])
		if width == 32 {
			a.offset, a.size, align = be.Uint64(h[8:]), be.Uint64(h[16:]), be.Uint32(h[24:])
			// Ignore reserved: Apple's lipo can leave this word uninitialized.
		}
		if a.offset < tableEnd || a.offset > uint64(len(b)) || a.size == 0 || a.size > uint64(len(b))-a.offset || align >= 64 || a.offset&((uint64(1)<<align)-1) != 0 {
			return nil, fmt.Errorf("universal slice %d has invalid range/alignment", i)
		}
		key := uint64(a.cpu)<<32 | uint64(a.subcpu)
		if seen[key] {
			return nil, fmt.Errorf("duplicate universal CPU/subtype")
		}
		seen[key], arches[i] = true, a
	}
	ranges := append([]arch(nil), arches...)
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].offset < ranges[j].offset })
	for i := 1; i < len(ranges); i++ {
		if ranges[i].offset < ranges[i-1].offset+ranges[i-1].size {
			return nil, fmt.Errorf("overlapping universal slices")
		}
	}
	f := &file{info: Info{Name: name, Format: "Mach-O", Kind: "universal"}}
	var kind string
	var imageType uint32
	for i, a := range arches {
		data := b[int(a.offset):int(a.offset+a.size)]
		sliceName := fmt.Sprintf("%s[%s:%#x]", name, machoArch(macho.Cpu(a.cpu)), a.subcpu)
		var child *file
		var err error
		if bytes.HasPrefix(data, []byte("!<arch>\n")) {
			entries, e := archiveEntries(data, false)
			if e != nil {
				return nil, e
			}
			for _, entry := range entries {
				cpu, subcpu, _, e := machoHeader(entry.data)
				if e != nil || cpu != a.cpu || subcpu != a.subcpu {
					return nil, fmt.Errorf("%s: archive member %s disagrees with universal CPU/subtype", sliceName, entry.name)
				}
			}
			child, err = parseArchive(sliceName, data)
			if err == nil {
				child.info.Arch, child.info.CPUSubtype = machoArch(macho.Cpu(a.cpu)), a.subcpu
				child.info.Bits = 32
				if a.cpu&0x1000000 != 0 {
					child.info.Bits = 64
				}
				child.info.OS = "darwin"
				for _, member := range child.members {
					if member.info.OS != "darwin" {
						child.info.OS = "apple-non-macos-archive"
					}
				}
			}
		} else {
			cpu, subcpu, typ, e := machoHeader(data)
			if e != nil || cpu != a.cpu || subcpu != a.subcpu {
				return nil, fmt.Errorf("%s: image disagrees with universal CPU/subtype or is not Mach-O", sliceName)
			}
			if i == 0 {
				imageType = typ
			} else if typ != imageType {
				return nil, fmt.Errorf("universal images have different Mach-O types")
			}
			child, err = parseMachO(sliceName, data)
		}
		if err != nil {
			return nil, err
		}
		if i == 0 {
			kind = child.info.Kind
		} else if kind != child.info.Kind {
			return nil, fmt.Errorf("universal slices have different file kinds")
		}
		f.slices = append(f.slices, child)
		f.info.Members = append(f.info.Members, child.info)
	}
	return f, nil
}

func machoHeader(b []byte) (cpu, subcpu, typ uint32, err error) {
	if len(b) < 16 {
		return 0, 0, 0, fmt.Errorf("truncated Mach-O header")
	}
	var order binary.ByteOrder
	switch binary.LittleEndian.Uint32(b) {
	case 0xfeedface, 0xfeedfacf:
		order = binary.LittleEndian
	case 0xcefaedfe, 0xcffaedfe:
		order = binary.BigEndian
	default:
		return 0, 0, 0, fmt.Errorf("not a Mach-O image")
	}
	return order.Uint32(b[4:]), order.Uint32(b[8:]), order.Uint32(b[12:]), nil
}

// Raw inputs select a baseline CPU subtype rather than guessing CPU features
// or pointer-authentication support. The OS loads the original universal dylib;
// require one host CPU variant so its selection cannot differ from validation.
func (f *file) hostSlice(goos, goarch string) (*file, error) {
	if goos != "darwin" || goarch != "amd64" && goarch != "arm64" {
		return nil, fmt.Errorf("%s: universal Mach-O is incompatible with host %s/%s", f.info.Name, goos, goarch)
	}
	var selected *file
	variants := 0
	for _, child := range f.slices {
		i := child.info
		if i.Arch != goarch {
			continue
		}
		variants++
		sub := i.CPUSubtype & 0xffffff
		if i.OS != "darwin" || i.Bits != 64 || goarch == "amd64" && sub != 3 || goarch == "arm64" && sub > 1 {
			continue
		}
		if selected == nil || sub < selected.info.CPUSubtype&0xffffff {
			selected = child
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("%s: no baseline macOS %s slice", f.info.Name, goarch)
	}
	if selected.info.Kind == "shared" && variants != 1 {
		return nil, fmt.Errorf("%s: universal shared library has multiple host CPU variants", f.info.Name)
	}
	return selected, nil
}
