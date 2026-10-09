package dylib

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestELF386GOTInstructionForms(t *testing.T) {
	for _, typ := range []uint32{3, 43} {
		for _, implicit := range []bool{false, true} {
			for _, tc := range []struct {
				name     string
				prefix   []byte
				absolute bool
			}{
				{"mov_eax", []byte{0x8b, 0x05}, true},
				{"mov_edx", []byte{0x8b, 0x15}, true},
				{"mov_edi", []byte{0x8b, 0x3d}, true},
				{"call", []byte{0xff, 0x15}, true},
				{"jump", []byte{0xff, 0x25}, true},
				{"based_mov", []byte{0x8b, 0x93}, false},
				{"based_call", []byte{0xff, 0x93}, false},
				{"lea", []byte{0x8d, 0x15}, false},
				{"no_prefix", nil, false},
				{"one_byte", []byte{0x15}, false},
			} {
				im, o := x64GOTImage(8, 0x22334455)
				o.info.Arch, im.pointerSize = "386", 4
				offset := len(tc.prefix)
				o.sections[1].size = uint64(offset + 4)
				o.sections[1].data = append(append([]byte{}, tc.prefix...), make([]byte, 4)...)
				copy(im.mem, o.sections[1].data)
				im.gotNext += 4
				addend := int64(7)
				if implicit {
					le.PutUint32(im.mem[offset:], 3)
				} else {
					le.PutUint32(im.mem[offset:], 0xcccccccc)
				}
				err := im.relocate(o, relocation{section: 1, offset: uint64(offset), typ: typ, addend: addend, implicit: implicit, pair: -1})
				want := uint32(11) // slot-GOT=4, plus explicit A=7.
				if implicit {
					want += 3
				}
				if tc.absolute {
					want += uint32(im.gotBase())
				}
				if err != nil || le.Uint32(im.mem[offset:]) != want || le.Uint32(im.mem[68:]) != 0x22334455 {
					t.Fatalf("%d %s (REL=%v): %#x, %v; want %#x", typ, tc.name, implicit, le.Uint32(im.mem[offset:]), err, want)
				}
				for i, b := range tc.prefix {
					if im.mem[i] != b {
						t.Fatal("GOT relocation rewrote its instruction")
					}
				}
			}
		}
	}
	// Relocation writes elsewhere in the image cannot change its input form.
	im, o := x64GOTImage(8, 0x22334455)
	o.info.Arch, im.pointerSize = "386", 4
	o.sections[1].data = []byte{0x8b, 0x15, 0, 0, 0, 0}
	copy(im.mem, o.sections[1].data)
	im.mem[0] = 0x8d
	if err := im.relocate(o, relocation{section: 1, offset: 2, typ: 3, pair: -1}); err != nil || le.Uint32(im.mem[2:]) != uint32(im.gotBase()) || im.mem[0] != 0x8d {
		t.Fatalf("GOT classification did not retain the original MOV context: %v", err)
	}
}

func TestELF386GOTMetadata(t *testing.T) {
	path := compile(t, "testdata/elf_386_got.s", filepath.Join(t.TempDir(), "got.o"), "--target=i386-linux-gnu")
	b, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := parse(path, b)
	if err != nil {
		t.Fatal(err)
	}
	absolute, relative := false, false
	for _, r := range f.obj.relocs {
		if r.typ != 3 && r.typ != 43 {
			continue
		}
		data := f.obj.sections[r.section].data
		if r.offset > 1 && data[r.offset-1]&0xc7 == 5 && data[r.offset-2] != 0x8d {
			absolute = true
		} else {
			relative = true
		}
	}
	if !absolute || !relative {
		t.Fatal("fixture lacks absolute or register-relative GOT forms")
	}
}

func TestNativeELF386GOTInstructionForms(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "386" {
		t.Skip("i386 GOT execution requires Linux 386")
	}
	needNative(t)
	dir := t.TempDir()
	clang := compile(t, "testdata/elf_386_got.s", filepath.Join(dir, "clang.o"))
	gnu := filepath.Join(dir, "gnu.o")
	command(t, "as", "--32", "-o", gnu, "testdata/elf_386_got.s")
	provider := compile(t, "testdata/elf_got_provider.c", filepath.Join(dir, "provider.o"))
	unused := compile(t, "testdata/missing.c", filepath.Join(dir, "unused.o"))
	archive := filepath.Join(dir, "provider.a")
	command(t, "ar", "rcs", archive, provider, unused)
	shared := filepath.Join(dir, "provider.so")
	command(t, compiler(), "-shared", "-fPIC", "testdata/elf_got_provider.c", "-o", shared)
	for _, consumer := range []string{clang, gnu} {
		label := filepath.Base(consumer)
		b, err := readFile(consumer)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parse(consumer, b)
		if err != nil {
			t.Fatal(err)
		}
		wantType := uint32(3)
		if consumer == gnu {
			wantType = 43
		}
		found := false
		for _, r := range f.obj.relocs {
			found = found || r.typ == wantType
		}
		if !found {
			t.Fatalf("%s did not emit GOT type %d", label, wantType)
		}
		rootsArchive := filepath.Join(dir, label+".a")
		command(t, "ar", "rcs", rootsArchive, consumer, provider, unused)
		for _, tc := range []struct {
			name   string
			inputs []string
			count  int
		}{
			{"objects", []string{consumer, provider}, 2},
			{"objects_reversed", []string{provider, consumer}, 2},
			{"archive", []string{consumer, archive}, 2},
			{"archive_roots", []string{rootsArchive}, 2},
			{"missing_retry", []string{consumer}, 2},
			{"shared_provider", []string{consumer, shared}, 1},
			{"defined_provider", []string{consumer}, 1},
		} {
			t.Run(label+"/"+tc.name, func(t *testing.T) {
				s := New(Options{})
				var external *Session
				defer func() {
					s.Close()
					if external != nil {
						external.Close()
					}
				}()
				load(t, s, tc.inputs...)
				if tc.name == "defined_provider" {
					external = New(Options{})
					load(t, external, provider)
					for _, name := range []string{"datum", "via_table"} {
						entry, err := external.Resolve(name)
						if err != nil {
							t.Fatal(err)
						}
						if err := entry.WithAddress(func(p uintptr) error { return s.Define(name, p) }); err != nil {
							t.Fatal(err)
						}
					}
				}
				if tc.name == "missing_retry" {
					if err := s.Link("got_abs_eval"); err == nil || s.image != nil {
						t.Fatal("missing provider was published")
					}
					load(t, s, archive)
				}
				if err := s.Link("got_abs_eval"); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"got_abs_eval", "got_based_eval", "got_lea_eval", "got_call_eval", "got_jump_eval"} {
					call(t, s, name, 20, 22, 84)
					call(t, s, name, -1, 22, 63)
				}
				call(t, s, "weak_abs_eval", 20, 22, 42)
				if len(s.image.objects) != tc.count {
					t.Fatalf("selected %d objects; want %d", len(s.image.objects), tc.count)
				}
			})
		}
		// GNU rejects baseless GOT loads in a DSO; compare a fixed executable.
		executable := filepath.Join(dir, label+".exe")
		command(t, compiler(), "-fno-pie", "-no-pie", "testdata/elf_386_got_main.c", "testdata/elf_got_provider.c", consumer, "-o", executable)
		command(t, executable)
	}
}
