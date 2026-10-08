package dylib

import (
	"bytes"
	"debug/pe"
	"fmt"
	"strconv"
	"strings"
)

var coffBigMagic = []byte{0xc7, 0xa1, 0xba, 0xd1, 0xee, 0xba, 0xa9, 0x4b, 0xaf, 0x20, 0xfa, 0xf6, 0x6a, 0xa4, 0xdc, 0xb8}

// debug/pe reads ordinary COFF/PE; bigobj needs 32-bit section numbers and
// 20-byte symbols. Both readers feed the same symbol and relocation logic.
type coffInput struct {
	machine   uint16
	timestamp uint32
	kind      string
	big       bool
	entrySize int
	records   []byte
	strings   pe.StringTable
	sections  []coffInputSection
}

type coffInputSection struct {
	name        string
	size, flags uint32
	relocs      []pe.Reloc
	data        []byte
	source      *pe.Section
}

func (s coffInputSection) contents() ([]byte, error) {
	if s.source != nil {
		return s.source.Data()
	}
	return s.data, nil
}

type coffInputSymbol struct {
	name       [8]byte
	value      uint32
	section    int32
	typ        uint16
	class, aux uint8
}

func (f *coffInput) record(i int) []byte { return f.records[i*f.entrySize : (i+1)*f.entrySize] }

func (f *coffInput) symbol(i int) coffInputSymbol {
	b := f.record(i)
	v := coffInputSymbol{value: le.Uint32(b[8:])}
	copy(v.name[:], b[:8])
	position := 14
	if f.big {
		v.section = int32(le.Uint32(b[12:]))
		position = 16
	} else {
		n := le.Uint16(b[12:])
		v.section = int32(n)
		if n > 65279 { // Reserved values, including ABSOLUTE=-1 and DEBUG=-2.
			v.section = int32(int16(n))
		}
	}
	v.typ, v.class, v.aux = le.Uint16(b[position:]), b[position+2], b[position+3]
	return v
}

func coffRange(b []byte, offset, size uint64) ([]byte, error) {
	if offset > uint64(len(b)) || size > uint64(len(b))-offset {
		return nil, fmt.Errorf("COFF table or section exceeds file bounds")
	}
	return b[int(offset):int(offset+size)], nil
}

func readCOFF(b []byte) (*coffInput, error) {
	if len(b) >= 4 && le.Uint32(b) == 0xffff0000 {
		return readBigCOFF(b)
	}
	f, err := pe.NewFile(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	v := &coffInput{machine: f.Machine, timestamp: f.TimeDateStamp, kind: "object", entrySize: 18, strings: f.StringTable}
	if f.OptionalHeader != nil {
		v.kind = "executable"
		if f.Characteristics&pe.IMAGE_FILE_DLL != 0 {
			v.kind = "shared"
		}
	}
	v.records, err = coffRange(b, uint64(f.PointerToSymbolTable), uint64(len(f.COFFSymbols))*18)
	if err != nil {
		return nil, err
	}
	for _, s := range f.Sections {
		sec := coffInputSection{name: s.Name, size: s.Size, flags: s.Characteristics, relocs: s.Relocs, source: s}
		if s.Characteristics&0x1000000 != 0 {
			sec.relocs, err = coffRelocations(b, s.PointerToRelocations, s.NumberOfRelocations, s.Characteristics)
			if err != nil {
				return nil, err
			}
		}
		v.sections = append(v.sections, sec)
	}
	return v, nil
}

func readBigCOFF(b []byte) (*coffInput, error) {
	if len(b) < 56 || le.Uint16(b[4:]) < 2 || !bytes.Equal(b[12:28], coffBigMagic) {
		return nil, fmt.Errorf("invalid bigobj header; COFF short import objects are not supported yet")
	}
	v := &coffInput{machine: le.Uint16(b[6:]), timestamp: le.Uint32(b[8:]), kind: "object", big: true, entrySize: 20}
	count, symoff, symbols := uint64(le.Uint32(b[44:])), uint64(le.Uint32(b[48:])), uint64(le.Uint32(b[52:]))
	headers, err := coffRange(b, 56, count*40)
	if err != nil {
		return nil, err
	}
	if count > 0x7fffffff || symbols != 0 && symoff == 0 {
		return nil, fmt.Errorf("invalid bigobj section or symbol table")
	}
	v.records, err = coffRange(b, symoff, symbols*20)
	if err != nil {
		return nil, err
	}
	if symoff != 0 {
		start := symoff + symbols*20
		size, err := coffRange(b, start, 4)
		if err != nil {
			return nil, err
		}
		length := uint64(le.Uint32(size))
		if length < 4 {
			return nil, fmt.Errorf("invalid bigobj string table size")
		}
		str, err := coffRange(b, start+4, length-4)
		if err != nil {
			return nil, err
		}
		v.strings = pe.StringTable(str)
	}
	v.sections = make([]coffInputSection, int(count))
	for i := range v.sections {
		h := headers[i*40 : (i+1)*40]
		name := strings.TrimRight(string(h[:8]), "\x00")
		if strings.HasPrefix(name, "/") {
			n, err := strconv.ParseUint(name[1:], 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid bigobj section name offset")
			}
			name, err = v.strings.String(uint32(n))
			if err != nil {
				return nil, err
			}
		}
		sec := coffInputSection{name: name, size: le.Uint32(h[16:]), flags: le.Uint32(h[36:])}
		if sec.flags&0x80 == 0 && sec.size != 0 {
			if le.Uint32(h[20:]) == 0 {
				return nil, fmt.Errorf("missing bigobj section data offset")
			}
			sec.data, err = coffRange(b, uint64(le.Uint32(h[20:])), uint64(sec.size))
			if err != nil {
				return nil, err
			}
		}
		sec.relocs, err = coffRelocations(b, le.Uint32(h[24:]), le.Uint16(h[32:]), sec.flags)
		if err != nil {
			return nil, err
		}
		v.sections[i] = sec
	}
	return v, nil
}

func coffRelocations(b []byte, offset uint32, count uint16, flags uint32) ([]pe.Reloc, error) {
	start, n := uint64(offset), uint64(count)
	if count != 0 && offset == 0 {
		return nil, fmt.Errorf("missing COFF relocation table offset")
	}
	if flags&0x1000000 != 0 {
		if count != 0xffff {
			return nil, fmt.Errorf("invalid COFF relocation overflow count")
		}
		marker, err := coffRange(b, start, 10)
		if err != nil {
			return nil, err
		}
		n = uint64(le.Uint32(marker))
		if n <= 0xffff || le.Uint32(marker[4:]) != 0 || le.Uint16(marker[8:]) != 0 {
			return nil, fmt.Errorf("invalid COFF relocation overflow marker")
		}
		start += 10
		n-- // The stored count includes the marker.
	}
	raw, err := coffRange(b, start, n*10)
	if err != nil {
		return nil, err
	}
	out := make([]pe.Reloc, int(n))
	for i := range out {
		r := raw[i*10:]
		out[i] = pe.Reloc{VirtualAddress: le.Uint32(r), SymbolTableIndex: le.Uint32(r[4:]), Type: le.Uint16(r[8:])}
	}
	return out, nil
}
