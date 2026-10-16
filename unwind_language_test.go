package dylib

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"
)

func frameWord(value uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, value)
	return b
}

func languageFrameImage(augmentation string, cieData, fdeData []byte) *image {
	im := elfUnwindTestImage()
	cie := append([]byte{1}, []byte(augmentation)...)
	cie = append(cie, 0, 1, 0x78, 16, byte(len(cieData)))
	cie = append(cie, cieData...)
	b := append(frameWord(uint32(4+len(cie))), make([]byte, 4)...)
	b = append(b, cie...)
	fde := append(frameWord(uint32(len(b)+4)), frameWord(uint32(im.base))...)
	fde = append(fde, frameWord(32)...)
	fde = append(fde, byte(len(fdeData)))
	fde = append(fde, fdeData...)
	b = append(b, frameWord(uint32(len(fde)))...)
	b = append(b, fde...)
	clear(im.mem[128:])
	copy(im.mem[128:], b)
	im.objects[0].sections[2].size = uint64(len(b))
	im.objects[0].sections = append(im.objects[0].sections,
		&section{name: ".gcc_except_table.test", offset: 384, size: 16, data: make([]byte, 16)},
		&section{name: ".data", offset: 416, size: 8, data: make([]byte, 8)},
	)
	im.gotStart, im.gotNext = 448, 456
	binary.LittleEndian.PutUint64(im.mem[448:], uint64(im.base))
	return im
}

func TestELFLanguageFrames(t *testing.T) {
	personality := append([]byte{3}, frameWord(0x100000)...)
	for _, test := range []struct {
		name, augmentation string
		cie, fde           []byte
	}{
		{"direct", "zPLR", append(append([]byte(nil), personality...), 3, 3), frameWord(0x100180)},
		{"reordered", "zRLP", append([]byte{3, 3}, personality...), frameWord(0x100180)},
		{"no_lsda", "zPR", append(append([]byte(nil), personality...), 3), nil},
		{"omitted_lsda", "zPLR", append(append([]byte(nil), personality...), 0xff, 3), nil},
		{"null_lsda", "zPLR", append(append([]byte(nil), personality...), 0x1b, 3), frameWord(0)},
		{"indirect_got", "zPLR", append(append([]byte{0x83}, frameWord(0x1001c0)...), 3, 3), frameWord(0x100180)},
	} {
		t.Run(test.name, func(t *testing.T) {
			im := languageFrameImage(test.augmentation, test.cie, test.fde)
			for _, size := range []uint64{4, 8} {
				im.pointerSize = size
				if frames, err := im.elfFrameSections(); err != nil || len(frames) != 1 {
					t.Fatalf("ELF%d: %v, %v", size*8, frames, err)
				}
			}
			if _, err := im.validateDWARFFrames(im.mem[128:128+im.objects[0].sections[2].size], 128, false, nil); err == nil {
				t.Fatal("C-only backend accepted language augmentation")
			}
		})
	}
	for _, test := range []struct {
		name, augmentation string
		cie, fde           []byte
		patch              func(*image)
		want               string
	}{
		{"duplicate", "zRR", []byte{3, 3}, nil, nil, "duplicate"},
		{"unknown", "zS", nil, nil, nil, "unsupported CIE"},
		{"missing_personality", "zLR", []byte{3, 3}, frameWord(0x100180), nil, "requires"},
		{"null_personality", "zPR", []byte{3, 0, 0, 0, 0, 3}, nil, nil, "null CIE"},
		{"data_personality", "zPR", append(append([]byte{3}, frameWord(0x1001a0)...), 3), nil, nil, "not executable"},
		{"external_slot", "zPR", append(append([]byte{0x83}, frameWord(0x200000)...), 3), nil, nil, "outside image"},
		{"unsupported_encoding", "zPR", []byte{0x11, 1, 3}, nil, nil, "unsupported frame"},
		{"truncated_personality", "zP", []byte{3, 1}, nil, nil, "truncated"},
		{"extra_cie_data", "zPR", append(append([]byte(nil), personality...), 3, 0), nil, nil, "unused CIE"},
		{"extra_fde_data", "zPR", append(append([]byte(nil), personality...), 3), []byte{0}, nil, "unused FDE"},
		{"truncated_lsda", "zPLR", append(append([]byte(nil), personality...), 3, 3), []byte{1}, nil, "truncated"},
		{"external_lsda", "zPLR", append(append([]byte(nil), personality...), 3, 3), frameWord(0x200000), nil, "outside read-only"},
		{"writable_lsda", "zPLR", append(append([]byte(nil), personality...), 3, 3), frameWord(0x100180), func(im *image) { im.objects[0].sections[3].write = true }, "outside read-only"},
		{"executable_lsda", "zPLR", append(append([]byte(nil), personality...), 3, 3), frameWord(0x100180), func(im *image) { im.objects[0].sections[3].exec = true }, "outside read-only"},
		{"unbacked_lsda", "zPLR", append(append([]byte(nil), personality...), 3, 3), frameWord(0x100180), func(im *image) { im.objects[0].sections[3].data = nil }, "outside read-only"},
		{"wrong_lsda_section", "zPLR", append(append([]byte(nil), personality...), 3, 3), frameWord(0x100180), func(im *image) { im.objects[0].sections[3].name = ".rodata" }, "outside read-only"},
	} {
		t.Run(test.name, func(t *testing.T) {
			im := languageFrameImage(test.augmentation, test.cie, test.fde)
			if test.patch != nil {
				test.patch(im)
			}
			if _, err := im.elfFrameSections(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want %q, got %v", test.want, err)
			}
		})
	}
}

func TestELFFramePointerBases(t *testing.T) {
	im := languageFrameImage("zR", []byte{3}, nil)
	for _, size := range []uint64{4, 8} {
		im.pointerSize = size
		for _, encoding := range []byte{0x1b, 0x9b} {
			r := frameReader{b: frameWord(0)}
			if got, err := im.framePointer(&r, encoding, 200); err != nil || got != 0 {
				t.Fatalf("encoded null: %x, %v", got, err)
			}
		}
		r := frameReader{b: frameWord(uint32(448 - 200))}
		if got, err := im.framePointer(&r, 0x9b, 200); err != nil || got != uint64(im.base) {
			t.Fatalf("PC-relative GOT: %x, %v", got, err)
		}
		r = frameReader{b: frameWord(0xfffffff0)}
		if got, err := im.framePointer(&r, 0x1b, 200); err != nil || got != uint64(im.base)+184 {
			t.Fatalf("signed PC-relative: %x, %v", got, err)
		}
	}
}

func TestELFCXXProducerMetadata(t *testing.T) {
	for _, triple := range []string{"x86_64-linux-gnu", "aarch64-linux-gnu", "i686-linux-gnu"} {
		t.Run(triple, func(t *testing.T) {
			var inputs []*object
			for _, kind := range []string{"throw", "catch"} {
				path := compile(t, "testdata/unwind_cxx_"+kind+".cpp", filepath.Join(t.TempDir(), kind+".o"), "--target="+triple, "-std=c++17", "-fPIC", "-fexceptions")
				data, err := readFile(path)
				if err != nil {
					t.Fatal(err)
				}
				f, err := parse(path, data)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, section := range f.obj.sections {
					if section != nil && elfLSDASection(section.name) {
						found = true
					}
				}
				if !found {
					t.Fatal("compiler LSDA missing")
				}
				inputs = append(inputs, f.obj)
			}
			for _, order := range [][]*object{inputs, {inputs[1], inputs[0]}, inputs} {
				before := len(order[1].relocs)
				coalesced, err := coalesceObjects(order)
				if err != nil {
					t.Fatal(err)
				}
				if len(coalesced[1].relocs) >= before || len(order[1].relocs) != before {
					t.Fatal("COMDAT frame pruning or input retry snapshot failed")
				}
			}
			for _, input := range inputs {
				discardUnwind(input)
				for _, section := range input.sections {
					if section != nil && (elfUnwindSection(section.name) || elfLSDASection(section.name)) {
						t.Fatal("default retained exception metadata")
					}
				}
				for _, relocation := range input.relocs {
					if input.sections[relocation.section] == nil {
						t.Fatal("default retained exception dependency")
					}
				}
			}
		})
	}
}
