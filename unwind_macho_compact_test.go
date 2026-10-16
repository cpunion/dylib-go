package dylib

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"
)

func compactTestImage() *image {
	o := &object{info: Info{Format: "Mach-O", Arch: "arm64", Bits: 64}, sections: []*section{
		nil,
		{name: "__text", offset: 4096, size: 256, exec: true},
		{name: "__compact_unwind", offset: 8192, size: 32, data: make([]byte, 32)},
		{name: "__gcc_except_tab", offset: 12288, size: 32, data: make([]byte, 32)},
	}}
	im := &image{mem: make([]byte, 20480), base: 0x100000, pointerSize: 8, page: 4096,
		objects: []*object{o}, machoHeader: true, compactStart: 16384, compactSize: 4096}
	le := binary.LittleEndian
	b := im.mem[8192:8224]
	le.PutUint64(b, uint64(im.base)+4096)
	le.PutUint32(b[8:], 64)
	le.PutUint32(b[12:], 0x44000001)
	le.PutUint64(b[16:], 0x900000)
	le.PutUint64(b[24:], uint64(im.base)+12288)
	copy(o.sections[2].data, b)
	return im
}

func TestMachOCompactCompilerRecords(t *testing.T) {
	for _, triple := range []string{"x86_64-apple-macosx11", "arm64-apple-macosx11"} {
		for _, optimization := range []string{"-O0", "-O2"} {
			for _, mode := range []string{"frame", "frameless", "large_stack"} {
				t.Run(triple+"/"+optimization+"/"+mode, func(t *testing.T) {
					flags := []string{"--target=" + triple, "-x", "c++", "-std=c++17", "-ffreestanding", optimization}
					if mode != "frame" {
						flags = append(flags, "-fomit-frame-pointer")
					}
					if mode == "large_stack" {
						flags = append(flags, "-DDYLIB_CXX_LARGE_STACK")
					}
					path := compile(t, "testdata/unwind_compact.cpp", filepath.Join(t.TempDir(), "compact.o"), flags...)
					data, err := readFile(path)
					if err != nil {
						t.Fatal(err)
					}
					file, err := parse(path, data)
					if err != nil {
						t.Fatal(err)
					}
					o := file.obj
					defs, err := definitions([]*object{o})
					if err != nil {
						t.Fatal(err)
					}
					got := uint64(len(o.sections) * 4096)
					im := &image{base: 0x100000, pointerSize: 8, page: 4096, objects: []*object{o}, defs: defs,
						mem: make([]byte, got+12288), got: map[uintptr]uintptr{}, stubs: map[uintptr]uintptr{}, resolved: map[string]uintptr{},
						gotStart: got, gotNext: got, stubStart: got + 4096, stubNext: got + 4096,
						machoHeader: true, compactStart: got + 8192, compactSize: 4096, external: func(string) uintptr { return 0x900000 }}
					var compact *section
					for index, s := range o.sections {
						if s == nil {
							continue
						}
						s.offset = uint64(index * 4096)
						copy(im.mem[s.offset:s.offset+s.size], s.data)
						if s.name == "__compact_unwind" {
							compact = s
						}
					}
					if compact == nil || compact.size == 0 {
						t.Fatal("compiler compact metadata missing")
					}
					original := append([]byte(nil), compact.data...)
					if count, err := im.machOCompactCount(); err != nil || count == 0 {
						t.Fatal(count, err)
					}
					for _, relocation := range o.relocs {
						if err := im.relocate(o, relocation); err != nil {
							t.Fatal(err)
						}
					}
					if err := im.rebaseMachODWARF(); err != nil {
						t.Fatal(err)
					}
					var dwarf []machoUnwindRange
					for _, s := range o.sections {
						if s == nil || s.name != "__eh_frame" {
							continue
						}
						policy := framePolicy{pcRelative: true, language: true, record: func(start, length uint64, _ uintptr, personality bool) error {
							dwarf = append(dwarf, machoUnwindRange{start: uintptr(start), end: uintptr(start + length), dwarf: im.base + uintptr(s.offset), size: uintptr(s.size), personality: personality})
							return nil
						}}
						if _, err := im.validateFrameRecords(im.mem[s.offset:s.offset+s.size], s.offset, policy); err != nil {
							t.Fatal(err)
						}
					}
					if ranges, err := im.machOCompactRanges(dwarf); err != nil || len(ranges) == 0 {
						t.Fatal(ranges, err)
					}
					if !bytes.Equal(original, compact.data) {
						t.Fatal("compiler compact source changed")
					}
					discardUnwind(o)
					for _, s := range o.sections {
						if s != nil && (s.name == "__compact_unwind" || s.name == "__eh_frame" || s.name == "__gcc_except_tab") {
							t.Fatal("default load retained unwind dependencies")
						}
					}
				})
			}
		}
	}
}

func TestMachOCompactTableAndOwnership(t *testing.T) {
	im := compactTestImage()
	original := append([]byte(nil), im.objects[0].sections[2].data...)
	if count, err := im.machOCompactCount(); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	ranges, err := im.machOCompactRanges(nil)
	if err != nil || len(ranges) != 1 {
		t.Fatal(ranges, err)
	}
	r := ranges[0]
	le := binary.LittleEndian
	table := im.mem[16384:16464]
	if r.start != im.base+4096 || r.end != im.base+4160 || r.compact != im.base+16384 || r.compactSize != 80 || r.dwarf != 0 {
		t.Fatal("native range", r)
	}
	if le.Uint32(im.mem) != 0xfeedfacf || le.Uint32(im.mem[4:]) != 0x0100000c || le.Uint32(table) != 1 || le.Uint32(table[24:]) != 2 {
		t.Fatal("native header/index")
	}
	if le.Uint32(table[28:]) != 8208 || le.Uint64(im.mem[8208:]) != 0x900000 {
		t.Fatal("personality must address the retained native pointer cell")
	}
	if le.Uint32(table[32:]) != 4096 || le.Uint32(table[44:]) != 4160 || le.Uint32(table[56:]) != 2 || le.Uint16(table[60:]) != 8 || le.Uint16(table[62:]) != 1 {
		t.Fatal("regular page range/sentinel")
	}
	if le.Uint32(table[68:]) != 0x54000001 || le.Uint32(table[72:]) != 4096 || le.Uint32(table[76:]) != 12288 || le.Uint32(table[52:]) != 80 {
		t.Fatal("personality index/LSDA table")
	}
	if !bytes.Equal(original, im.objects[0].sections[2].data) {
		t.Fatal("compact table construction changed staged source")
	}
	// Ordinary C frames retain FDE lookup; language compact entries must not be
	// hidden by redundant DWARF emitted by x86-64 Clang.
	im = compactTestImage()
	le.PutUint64(im.mem[8208:], 0)
	le.PutUint64(im.mem[8216:], 0)
	le.PutUint32(im.mem[8204:], 0x04000001)
	dwarf := machoUnwindRange{start: im.base + 4096, end: im.base + 4160, dwarf: im.base + 10000, size: 80, personality: true}
	ranges, err = im.machOCompactRanges([]machoUnwindRange{dwarf})
	if err != nil || len(ranges) != 1 || ranges[0] != dwarf || im.compactUsed != 0 {
		t.Fatal("existing DWARF ownership changed", ranges, err)
	}
	im = compactTestImage()
	ranges, err = im.machOCompactRanges([]machoUnwindRange{dwarf})
	if err != nil || len(ranges) != 1 || ranges[0].compact == 0 || im.compactUsed != 80 {
		t.Fatal("native language compact entry hidden by redundant DWARF", ranges, err)
	}
	im = compactTestImage()
	le.PutUint32(im.mem[8204:], 0x03000000)
	ranges, err = im.machOCompactRanges([]machoUnwindRange{dwarf})
	if err != nil || len(ranges) != 1 || ranges[0] != dwarf || im.compactUsed != 0 {
		t.Fatal("explicit DWARF-mode fallback changed", ranges, err)
	}
}

func TestMachOCompactRejectsMalformedMetadata(t *testing.T) {
	le := binary.LittleEndian
	for name, patch := range map[string]func(*image){
		"storage_write":     func(im *image) { im.objects[0].sections[2].write = true },
		"storage_exec":      func(im *image) { im.objects[0].sections[2].exec = true },
		"partial_record":    func(im *image) { im.objects[0].sections[2].size = 31 },
		"missing_bytes":     func(im *image) { im.objects[0].sections[2].data = nil },
		"pointer_width":     func(im *image) { im.pointerSize = 4 },
		"function_address":  func(im *image) { le.PutUint64(im.mem[8192:], 0x900000) },
		"zero_length":       func(im *image) { le.PutUint32(im.mem[8200:], 0) },
		"function_extent":   func(im *image) { le.PutUint32(im.mem[8200:], 257) },
		"personality_index": func(im *image) { le.PutUint32(im.mem[8204:], 0x54000001) },
		"LSDA_address":      func(im *image) { le.PutUint64(im.mem[8216:], 0x900000) },
		"LSDA_storage":      func(im *image) { im.objects[0].sections[3].write = true },
		"LSDA_null":         func(im *image) { le.PutUint64(im.mem[8216:], 0) },
		"reserved_encoding": func(im *image) { le.PutUint32(im.mem[8204:], 0x44000020) },
		"unrecognized_mode": func(im *image) { le.PutUint32(im.mem[8204:], 0x49000000) },
		"DWARF_missing":     func(im *image) { le.PutUint32(im.mem[8204:], 0x03000000) },
		"duplicate": func(im *image) {
			im.objects[0].sections[2].size = 64
			im.objects[0].sections[2].data = make([]byte, 64)
			copy(im.mem[8224:], im.mem[8192:8224])
		},
	} {
		t.Run(name, func(t *testing.T) {
			im := compactTestImage()
			patch(im)
			_, err := im.machOCompactCount()
			if err == nil {
				_, err = im.machOCompactRanges(nil)
			}
			if err == nil {
				t.Fatal("malformed compact unwind metadata accepted")
			}
		})
	}
}

func TestMachOCompactEncodingBounds(t *testing.T) {
	for _, test := range []struct {
		arch     string
		encoding uint32
		valid    bool
	}{
		{"arm64", 0x04000f1f, true}, {"arm64", 0x02001000, true}, {"arm64", 0x04000020, false},
		{"amd64", 0x01010001, true}, {"amd64", 0x01010006, false}, {"amd64", 0x01000001, false},
		{"amd64", 0x02010000, true}, {"amd64", 0x02000000, false}, {"amd64", 0x02021c00, false},
		{"amd64", 0x02020405, true}, {"amd64", 0x02020406, false}, {"amd64", 0x03030000, true},
	} {
		if err := validateCompactEncoding(test.arch, test.encoding); (err == nil) != test.valid {
			t.Errorf("%s %#x validity %v: %v", test.arch, test.encoding, test.valid, err)
		}
	}
}

// The indirect encoding reads an instruction immediate, so validate the byte
// range and instruction form before exposing a table to the native unwinder.
func TestMachOCompactIndirectStackInstruction(t *testing.T) {
	for _, name := range []string{"valid", "short_function", "short_offset", "opcode", "negative_allocation", "insufficient_stack"} {
		t.Run(name, func(t *testing.T) {
			im := compactTestImage()
			im.objects[0].info.Arch = "amd64"
			le := binary.LittleEndian
			le.PutUint32(im.mem[8204:], 0x43030000)
			copy(im.mem[4096:], []byte{0x48, 0x81, 0xec, 0x00, 0x20, 0x00, 0x00})
			switch name {
			case "short_function":
				le.PutUint32(im.mem[8200:], 6)
			case "short_offset":
				le.PutUint32(im.mem[8204:], 0x43020000)
			case "opcode":
				im.mem[4098] = 0xc4
			case "negative_allocation":
				le.PutUint32(im.mem[4099:], 0x80000000)
			case "insufficient_stack":
				le.PutUint32(im.mem[4099:], 0)
			}
			_, err := im.machOCompactRanges(nil)
			if (err == nil) != (name == "valid") {
				t.Fatal(name, err)
			}
		})
	}
}
