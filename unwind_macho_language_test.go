package dylib

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"
)

// Relocate actual compiler metadata in Go-owned storage. Fake runtime addresses
// are only written to GOT cells; this test never calls them or registers frames.
func machoLanguageImage(t *testing.T, input *object) (*image, *section) {
	t.Helper()
	objects, err := coalesceObjects([]*object{input})
	if err != nil {
		t.Fatal(err)
	}
	defs, err := definitions(objects)
	if err != nil {
		t.Fatal(err)
	}
	o := objects[0]
	got := uint64(len(o.sections) * 4096)
	im := &image{base: 0x100000, pointerSize: 8, objects: objects, defs: defs,
		mem: make([]byte, got+8192), external: func(string) uintptr { return 0x800000 },
		resolved: map[string]uintptr{}, got: map[uintptr]uintptr{}, stubs: map[uintptr]uintptr{},
		gotStart: got, gotNext: got, stubStart: got + 4096, stubNext: got + 4096}
	var frame *section
	for index, section := range o.sections {
		if section == nil {
			continue
		}
		section.offset = uint64(index * 4096)
		copy(im.mem[section.offset:section.offset+section.size], section.data)
		if section.name == "__eh_frame" {
			frame = section
		}
	}
	if frame == nil || !bytes.Contains(frame.data, []byte("zPLR")) {
		t.Fatal("compiler language DWARF missing")
	}
	for _, relocation := range o.relocs {
		if err := im.relocate(o, relocation); err != nil {
			t.Fatal(err)
		}
	}
	return im, frame
}

func TestMachOCXXProducerAndRebase(t *testing.T) {
	for _, target := range []string{"x86_64-apple-macosx11", "arm64-apple-macosx11"} {
		t.Run(target, func(t *testing.T) {
			for _, optimization := range []string{"-O0", "-O2"} {
				t.Run(optimization, func(t *testing.T) {
					path := compile(t, "testdata/unwind_cxx_catch.cpp", filepath.Join(t.TempDir(), "catch.o"), "--target="+target, optimization, "-std=c++17", "-fexceptions", "-femit-dwarf-unwind=always")
					data, err := readFile(path)
					if err != nil {
						t.Fatal(err)
					}
					f, err := parse(path, data)
					if err != nil || len(f.info.Unsupported) != 0 {
						t.Fatalf("C++ producer: %v, %v", f, err)
					}
					for range 2 {
						im, frame := machoLanguageImage(t, f.obj)
						original := append([]byte(nil), frame.data...)
						if err := im.rebaseMachODWARF(); err != nil {
							t.Fatal(err)
						}
						fdes, err := im.validateFrameRecords(im.mem[frame.offset:frame.offset+frame.size], frame.offset, framePolicy{pcRelative: true, language: true})
						if err != nil || len(fdes) < 2 {
							t.Fatalf("rebased language FDEs: %v, %v", fdes, err)
						}
						if !bytes.Equal(frame.data, original) {
							t.Fatal("rebasing changed staged input")
						}
					}
					mutations := []string{"writable_lsda", "missing_lsda", "unbacked_lsda", "external_personality_slot"}
					if f.obj.info.Arch == "amd64" {
						mutations = append(mutations, "ambiguous_lsda")
					}
					for _, mutation := range mutations {
						t.Run(mutation, func(t *testing.T) {
							im, frame := machoLanguageImage(t, f.obj)
							for _, section := range im.objects[0].sections {
								if section != nil && section.name == "__gcc_except_tab" {
									if mutation == "writable_lsda" {
										section.write = true
									}
									if mutation == "missing_lsda" {
										section.name = "__rodata"
									}
									if mutation == "unbacked_lsda" {
										section.data = nil
									}
									if mutation == "ambiguous_lsda" {
										duplicate := *section
										im.objects[0].sections = append(im.objects[0].sections, &duplicate)
									}
								}
							}
							if mutation == "external_personality_slot" {
								// A CIE cannot dereference arbitrary external memory.
								for _, r := range im.objects[0].relocs {
									if im.objects[0].sections[r.section] == frame && r.pcrel {
										copy(im.mem[frame.offset+r.offset:frame.offset+r.offset+4], []byte{0xff, 0xff, 0xff, 0x7f})
									}
								}
							}
							if err := im.rebaseMachODWARF(); err == nil {
								t.Fatal("invalid language metadata accepted")
							}
						})
					}
					discardUnwind(f.obj)
					for _, section := range f.obj.sections {
						if section != nil && (section.name == "__eh_frame" || lsdaSection("Mach-O", section.name)) {
							t.Fatal("default retained language metadata")
						}
					}
				})
			}
		})
	}
}

func TestMachONullAndInvalidLSDAReference(t *testing.T) {
	im := languageFrameImage("zPLR", append(append([]byte{3}, frameWord(0x100000)...), 0x1b, 0x1b), frameWord(0))
	o := im.objects[0]
	o.info.Format = "Mach-O"
	o.sections[2].name, o.sections[3].name = "__eh_frame", "__gcc_except_tab"
	for _, section := range o.sections {
		if section != nil {
			section.original = section.offset
		}
	}
	fde := uint64(128 + 4 + binary.LittleEndian.Uint32(im.mem[128:]))
	code := fde + 8
	binary.LittleEndian.PutUint32(im.mem[code:], uint32(-int32(code)))
	if err := im.rebaseMachODWARF(); err != nil {
		t.Fatal(err)
	}
	lsda := code + 9 // fixed 4-byte PC, 4-byte range, 1-byte augmentation length
	if binary.LittleEndian.Uint32(im.mem[lsda:]) != 0 {
		t.Fatal("null LSDA rebased")
	}
	binary.LittleEndian.PutUint32(im.mem[lsda:], uint32(0x999-int64(lsda)))
	if err := im.rebaseMachODWARF(); err == nil {
		t.Fatal("implicit LSDA outside original data accepted")
	}
}

func TestMachOARM64GOTPointerPlaceholder(t *testing.T) {
	for _, word := range []uint32{0, 0xbfffffed, 0x7fffffff, 0xffffffff} {
		im, o := armGOTImage(4, 0x22334455)
		o.info.Format = "Mach-O"
		le.PutUint32(im.mem, word)
		if err := im.relocate(o, relocation{section: 1, typ: 7, width: 4, pcrel: true, pair: -1}); err != nil {
			t.Fatal(err)
		}
		if le.Uint32(im.mem) != 64 || le.Uint64(im.mem[64:]) != 0x22334455 {
			t.Fatal("placeholder affected GOT slot or pointer displacement")
		}
	}
}
