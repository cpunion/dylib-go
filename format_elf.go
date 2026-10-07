package dylib

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

func parseELF(name string, b []byte) (*file, error) {
	f, err := elf.NewFile(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	o := &object{info: Info{Name: name, Format: "ELF", Kind: "object", Bits: 64}}
	if f.OSABI == elf.ELFOSABI_LINUX {
		o.info.OS = "linux"
	} else if f.OSABI != elf.ELFOSABI_NONE {
		o.info.OS = f.OSABI.String()
	}
	if f.Class != elf.ELFCLASS64 {
		o.info.Bits = 32
		if f.Machine != elf.EM_386 {
			o.unsupported("ELF32 machine without a relocation backend")
		}
	}
	if f.ByteOrder != binary.LittleEndian {
		o.unsupported("big-endian relocation")
	}
	switch f.Machine {
	case elf.EM_X86_64:
		o.info.Arch = "amd64"
	case elf.EM_AARCH64:
		o.info.Arch = "arm64"
	case elf.EM_386:
		o.info.Arch = "386"
	default:
		o.info.Arch = f.Machine.String()
		o.unsupported("machine " + f.Machine.String())
	}
	if f.Type == elf.ET_DYN {
		o.info.Kind = "shared"
		for _, p := range f.Progs {
			if p.Type == elf.PT_INTERP {
				o.info.Kind = "executable"
			}
		}
		flags, _ := f.DynValue(elf.DT_FLAGS_1)
		for _, v := range flags {
			if v&0x08000000 != 0 {
				o.info.Kind = "executable"
			}
		}
	} else if f.Type != elf.ET_REL {
		o.info.Kind = "executable"
	}
	syms, err := f.Symbols()
	if errors.Is(err, elf.ErrNoSymbols) {
		syms, err = f.DynamicSymbols()
	}
	if err != nil && !errors.Is(err, elf.ErrNoSymbols) {
		return nil, err
	}
	o.symbols = append(o.symbols, symbol{}) // ELF omits the null symbol in Symbols().
	for _, s := range syms {
		v := symbol{name: s.Name, section: int(s.Section), value: s.Value, size: s.Size, global: elf.ST_BIND(s.Info) != elf.STB_LOCAL, weak: elf.ST_BIND(s.Info) == elf.STB_WEAK}
		switch s.Section {
		case elf.SHN_UNDEF:
			v.section = 0
		case elf.SHN_ABS:
			v.section = -1
		case elf.SHN_COMMON:
			v.section = -2
			v.align = s.Value
			v.value = 0
		}
		if elf.ST_TYPE(s.Info) == elf.STT_TLS {
			o.unsupported("TLS symbol " + s.Name)
		}
		if elf.ST_TYPE(s.Info) == elf.STT_GNU_IFUNC {
			o.unsupported("GNU IFUNC symbol " + s.Name)
		}
		o.symbols = append(o.symbols, v)
	}
	if o.info.Kind != "object" {
		return finish(o), nil
	}
	o.sections = make([]*section, len(f.Sections))
	for i, s := range f.Sections {
		if s.Flags&elf.SHF_TLS != 0 {
			o.unsupported("TLS section " + s.Name)
		}
		if s.Flags&elf.SHF_ALLOC == 0 || strings.HasPrefix(s.Name, ".eh_frame") || s.Name == ".gcc_except_table" {
			continue
		}
		if strings.HasPrefix(s.Name, ".ctors") || strings.HasPrefix(s.Name, ".dtors") || s.Name == ".init" || s.Name == ".fini" {
			o.unsupported("automatic initialization/finalization: " + s.Name)
		}
		v := &section{name: s.Name, size: s.Size, align: s.Addralign, write: s.Flags&elf.SHF_WRITE != 0, exec: s.Flags&elf.SHF_EXECINSTR != 0}
		switch s.Type {
		case elf.SHT_PREINIT_ARRAY:
			v.lifecycle = lifecyclePreinit
		case elf.SHT_INIT_ARRAY:
			v.lifecycle = lifecycleInit
		case elf.SHT_FINI_ARRAY:
			v.lifecycle = lifecycleFini
		}
		if v.lifecycle != 0 {
			if err := configureLifecycle(v, o.info.Bits); err != nil {
				return nil, err
			}
		}
		if err := validSection(v); err != nil {
			return nil, err
		}
		if s.Type != elf.SHT_NOBITS {
			v.data, err = s.Data()
			if err != nil {
				return nil, err
			}
		}
		o.sections[i] = v
	}
	grouped := make(map[int]bool)
	for _, s := range f.Sections {
		if s.Type != elf.SHT_GROUP {
			continue
		}
		if uint64(s.Link) >= uint64(len(f.Sections)) || f.Sections[s.Link].Type != elf.SHT_SYMTAB || uint64(s.Info) >= uint64(len(o.symbols)) {
			return nil, fmt.Errorf("invalid ELF group symbol table or signature")
		}
		data, err := s.Data()
		if err != nil {
			return nil, err
		}
		if len(data) < 8 || len(data)%4 != 0 || f.ByteOrder.Uint32(data) != 1 {
			return nil, fmt.Errorf("only nonempty ELF GRP_COMDAT groups are supported")
		}
		g := sectionGroup{key: o.symbols[s.Info].name, selection: 2}
		if g.key == "" {
			return nil, fmt.Errorf("empty ELF COMDAT signature")
		}
		for offset := 4; offset < len(data); offset += 4 {
			index := int(f.ByteOrder.Uint32(data[offset:]))
			if index <= 0 || index >= len(f.Sections) || f.Sections[index].Type == elf.SHT_GROUP || grouped[index] || f.Sections[index].Flags&elf.SHF_GROUP == 0 {
				return nil, fmt.Errorf("invalid or duplicate ELF COMDAT member")
			}
			grouped[index] = true
			g.sections = append(g.sections, index)
		}
		o.groups = append(o.groups, g)
	}
	for i, s := range f.Sections {
		if s.Flags&elf.SHF_GROUP != 0 && !grouped[i] {
			return nil, fmt.Errorf("ELF grouped section has no group")
		}
	}
	for _, s := range f.Sections {
		if s.Type != elf.SHT_RELA && s.Type != elf.SHT_REL {
			continue
		}
		if uint64(s.Info) >= uint64(len(o.sections)) {
			return nil, fmt.Errorf("invalid relocation section target")
		}
		if o.sections[s.Info] == nil {
			continue
		}
		is386 := f.Class == elf.ELFCLASS32 && f.Machine == elf.EM_386
		if !is386 && (f.Class != elf.ELFCLASS64 || s.Type == elf.SHT_REL) {
			o.unsupported("only ELF64 RELA and i386 ELF32 REL/RELA are implemented")
			continue
		}
		if uint64(s.Link) >= uint64(len(f.Sections)) || f.Sections[s.Link].Type != elf.SHT_SYMTAB {
			return nil, fmt.Errorf("relocations require a regular symbol table")
		}
		data, e := s.Data()
		if e != nil {
			return nil, e
		}
		entrySize := 24
		if is386 {
			entrySize = 8
			if s.Type == elf.SHT_RELA {
				entrySize = 12
			}
		}
		if len(data)%entrySize != 0 {
			return nil, fmt.Errorf("invalid ELF relocation entry size")
		}
		for len(data) > 0 {
			if is386 {
				info := f.ByteOrder.Uint32(data[4:])
				si := uint64(info >> 8)
				if si >= uint64(len(o.symbols)) {
					return nil, fmt.Errorf("invalid relocation symbol %d", si)
				}
				r := relocation{section: int(s.Info), offset: uint64(f.ByteOrder.Uint32(data)), typ: info & 0xff, symbol: int(si), implicit: s.Type == elf.SHT_REL, pair: -1}
				if !r.implicit {
					r.addend = int64(int32(f.ByteOrder.Uint32(data[8:])))
				}
				o.relocs = append(o.relocs, r)
				data = data[entrySize:]
				continue
			}
			info := f.ByteOrder.Uint64(data[8:])
			si := uint64(info >> 32)
			if si >= uint64(len(o.symbols)) {
				return nil, fmt.Errorf("invalid relocation symbol %d", si)
			}
			o.relocs = append(o.relocs, relocation{section: int(s.Info), offset: f.ByteOrder.Uint64(data), typ: uint32(info), symbol: int(si), addend: int64(f.ByteOrder.Uint64(data[16:])), pair: -1})
			data = data[24:]
		}
	}
	return finish(o), nil
}
