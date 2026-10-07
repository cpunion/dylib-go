package dylib

import (
	"bytes"
	"debug/pe"
	"fmt"
	"strings"
)

func parseCOFF(name string, b []byte) (*file, error) {
	f, err := pe.NewFile(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	o := &object{info: Info{Name: name, Format: "COFF", Kind: "object", Bits: 64}}
	o.info.OS = "windows"
	switch f.Machine {
	case pe.IMAGE_FILE_MACHINE_AMD64:
		o.info.Arch = "amd64"
	case pe.IMAGE_FILE_MACHINE_ARM64:
		o.info.Arch = "arm64"
	case pe.IMAGE_FILE_MACHINE_I386:
		o.info.Arch = "386"
		o.info.Bits = 32
	default:
		o.info.Arch = fmt.Sprintf("machine-%x", f.Machine)
		o.info.Bits = 32
		o.unsupported("32-bit COFF relocation")
	}
	if f.OptionalHeader != nil {
		o.info.Format = "PE"
		o.info.Kind = "executable"
		if f.Characteristics&pe.IMAGE_FILE_DLL != 0 {
			o.info.Kind = "shared"
		}
	}
	o.symbols = make([]symbol, len(f.COFFSymbols))
	for i := 0; i < len(f.COFFSymbols); {
		s := f.COFFSymbols[i]
		n, e := s.FullName(f.StringTable)
		if e != nil {
			return nil, e
		}
		if o.info.Arch == "386" {
			// Normalize the x86 C linker underscore, retaining the import
			// marker and any stdcall suffix. Calling conventions stay explicit.
			if strings.HasPrefix(n, "__imp__") {
				n = "__imp_" + strings.TrimPrefix(n, "__imp__")
			} else {
				n = strings.TrimPrefix(n, "_")
			}
		}
		v := symbol{name: n, section: int(s.SectionNumber), value: uint64(s.Value), global: s.StorageClass == 2, weak: s.StorageClass == 105}
		if v.section == 0 && s.Value != 0 {
			v.section = -2
			v.size = uint64(s.Value)
			v.value = 0
			v.align = 16
		}
		if s.StorageClass == 105 {
			o.unsupported("COFF weak externals/alias auxiliary records")
		}
		o.symbols[i] = v
		if i+int(s.NumberOfAuxSymbols) >= len(f.COFFSymbols) {
			return nil, fmt.Errorf("invalid COFF auxiliary symbol count")
		}
		for j := 1; j <= int(s.NumberOfAuxSymbols); j++ {
			o.symbols[i+j].section = -3
		}
		i += 1 + int(s.NumberOfAuxSymbols)
	}
	if o.info.Kind != "object" {
		return finish(o), nil
	}
	o.sections = make([]*section, len(f.Sections)+1)
	for i, s := range f.Sections {
		flags := s.Characteristics
		if flags&0x200 != 0 || strings.HasPrefix(s.Name, ".debug") || s.Name == ".drectve" || s.Name == ".pdata" || s.Name == ".xdata" {
			continue
		}
		if flags&0x1000 != 0 {
			o.unsupported("COFF COMDAT section " + s.Name)
		}
		if strings.HasPrefix(s.Name, ".tls") {
			o.unsupported("TLS section " + s.Name)
		}
		if strings.HasPrefix(s.Name, ".CRT") {
			o.unsupported("automatic initialization/finalization: " + s.Name)
		}
		align := uint64(16)
		a := (flags >> 20) & 15
		if a != 0 {
			align = uint64(1) << (a - 1)
		}
		v := &section{name: s.Name, size: uint64(s.Size), align: align, exec: flags&0x20000000 != 0, write: flags&0x80000000 != 0}
		if err := validSection(v); err != nil {
			return nil, err
		}
		if flags&0x80 == 0 {
			v.data, err = s.Data()
			if err != nil {
				return nil, err
			}
		}
		o.sections[i+1] = v
		for _, r := range s.Relocs {
			o.relocs = append(o.relocs, relocation{section: i + 1, offset: uint64(r.VirtualAddress), typ: uint32(r.Type), symbol: int(r.SymbolTableIndex), pair: -1})
		}
	}
	return finish(o), nil
}
