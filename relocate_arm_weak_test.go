package dylib

import (
	"path/filepath"
	"runtime"
	"testing"
)

func weakARM64Image(width int) (*image, *object) {
	o := &object{info: Info{Format: "ELF", Arch: "arm64"}, sections: []*section{nil, {size: uint64(width)}}, symbols: []symbol{{name: "optional", global: true, weak: true}}}
	im := &image{base: 0x10000000, mem: make([]byte, 128), resolved: map[string]uintptr{}, external: func(string) uintptr { return 0 }, got: map[uintptr]uintptr{}, gotStart: 64, gotNext: 64, stubStart: 128, pointerSize: 8}
	return im, o
}

func TestARM64ELFWeakRelocations(t *testing.T) {
	for _, tc := range []struct {
		typ, width uint32
		ins        uint32
		addend     int64
		want       uint64
	}{
		{260, 8, 0, -7, ^uint64(6)}, {261, 4, 0, 42, 42}, {262, 2, 0, 42, 42},
		{273, 4, 0x18000002, 4, 0x18000022}, {274, 4, 0x10000002, 1, 0x30000002},
		{275, 4, 0x90000002, 4096, 0xb0000002}, {276, 4, 0x90000002, 4096, 0xb0000002},
		{279, 4, 0x36000002, 4, 0x36000022}, {280, 4, 0x5400000b, 4, 0x5400002b},
		{282, 4, 0x17ffffff, 16, 0xd503201f}, {283, 4, 0x97ffffff, -4, 0xd503201f},
		{287, 4, 0xd29fffe2, 42, 0xd2800542}, {288, 4, 0xf2800002, 42, 0xf2800542},
		{289, 4, 0xd2a00002, 0x12340000, 0xd2a24682}, {290, 4, 0xf2a00002, 0x12340000, 0xf2a24682},
		{291, 4, 0xd2c00002, -1, 0x92c00002}, {292, 4, 0xf2c00002, -1, 0xf2dfffe2},
		{293, 4, 0xd2e00002, -1, 0x92e00002},
		{257, 8, 0, 0, 0}, {258, 4, 0, 42, 42}, {259, 2, 0, 42, 42},
		{264, 4, 0xd29fffe2, 42, 0xd2800542}, {270, 4, 0x929fffe2, 42, 0xd2800542},
		{309, 4, 0x58000002, 0, 0x58000202}, // Slot address is relative, target stays zero.
	} {
		im, o := weakARM64Image(int(tc.width))
		if tc.width == 4 {
			le.PutUint32(im.mem, tc.ins)
		}
		err := im.relocate(o, relocation{section: 1, typ: tc.typ, addend: tc.addend, pair: -1})
		var got uint64
		switch tc.width {
		case 2:
			got = uint64(le.Uint16(im.mem))
		case 4:
			got = uint64(le.Uint32(im.mem))
		case 8:
			got = le.Uint64(im.mem)
		}
		if err != nil || got != tc.want || tc.typ == 309 && le.Uint64(im.mem[64:]) != 0 {
			t.Fatalf("weak relocation %d: %#x, %v; want %#x", tc.typ, got, err, tc.want)
		}
	}
	for _, section := range []int{-1, 0} {
		im, o := weakARM64Image(4)
		if section == -1 {
			o.symbols[0].section = -1 // Weak definition at absolute zero.
		} else {
			provider := &object{symbols: []symbol{{name: "optional", global: true, section: -1}}}
			im.defs = map[string]definition{"optional": {provider, 0}}
		}
		le.PutUint32(im.mem, 0x94000000)
		im.base = 0x100
		if err := im.relocate(o, relocation{section: 1, typ: 283, pair: -1}); err != nil || le.Uint32(im.mem) != 0x97ffffc0 {
			t.Fatalf("zero definition became a no-op: %#x, %v", le.Uint32(im.mem), err)
		}
	}
	im, o := weakARM64Image(4)
	im.external = func(string) uintptr { return im.base + 64 }
	le.PutUint32(im.mem, 0x94000000)
	if err := im.relocate(o, relocation{section: 1, typ: 283, pair: -1}); err != nil || le.Uint32(im.mem) != 0x94000010 {
		t.Fatalf("external weak target became a no-op: %#x, %v", le.Uint32(im.mem), err)
	}
	for _, tc := range []struct{ typ, ins uint32 }{{283, 0x14000000}, {282, 0x94000000}, {283, 0xd503201f}} {
		im, o := weakARM64Image(4)
		le.PutUint32(im.mem, tc.ins)
		if err := im.relocate(o, relocation{section: 1, typ: tc.typ, pair: -1}); err == nil || le.Uint32(im.mem) != tc.ins {
			t.Fatal("invalid weak branch accepted or changed")
		}
	}
	im, o = weakARM64Image(4)
	im.base++
	le.PutUint32(im.mem, 0x94000000)
	if err := im.relocate(o, relocation{section: 1, typ: 283, pair: -1}); err == nil || le.Uint32(im.mem) != 0x94000000 {
		t.Fatal("misaligned weak call accepted or changed")
	}
}

func TestARM64ELFWeakMetadata(t *testing.T) {
	for _, src := range []string{"elf_arm_weak_call.c", "elf_arm_weak_refs.s"} {
		path := compile(t, "testdata/"+src, filepath.Join(t.TempDir(), "weak.o"), "--target=aarch64-linux-gnu")
		b, err := readFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parse(path, b)
		if err != nil || len(f.info.Unsupported) != 0 {
			t.Fatalf("weak metadata: %v, %v", f, err)
		}
		found := false
		for _, r := range f.obj.relocs {
			v := f.obj.symbols[r.symbol]
			found = found || v.section == 0 && v.weak && (r.typ == 283 || r.typ == 262)
		}
		if !found {
			t.Fatal("fixture has no expected weak relocation")
		}
	}
}

func TestNativeARM64ELFWeakCalls(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" {
		t.Skip("AArch64 ELF execution requires Linux arm64")
	}
	needNative(t)
	dir := t.TempDir()
	consumer := compile(t, "testdata/elf_arm_weak_call.c", filepath.Join(dir, "consumer.o"))
	provider := compile(t, "testdata/elf_arm_weak_provider.c", filepath.Join(dir, "provider.o"))
	archive := filepath.Join(dir, "provider.a")
	command(t, "ar", "rcs", archive, provider)
	rootArchive := filepath.Join(dir, "root.a")
	command(t, "ar", "rcs", rootArchive, consumer, provider)
	shared := filepath.Join(dir, "provider.so")
	command(t, compiler(), "-shared", "-fPIC", "testdata/elf_arm_weak_provider.c", "-o", shared)
	for _, tc := range []struct {
		name     string
		inputs   []string
		provided bool
		objects  int
	}{
		{"absent", []string{consumer}, false, 1},
		{"unselected_archive", []string{consumer, archive}, false, 1},
		{"objects", []string{consumer, provider}, true, 2},
		{"objects_reversed", []string{provider, consumer}, true, 2},
		{"archive_root", []string{rootArchive}, false, 1},
		{"explicit_archive_root", []string{rootArchive}, true, 2},
		{"shared_provider", []string{consumer, shared}, true, 1},
		{"defined_provider", []string{consumer}, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
				entry, err := external.Resolve("optional_event")
				if err != nil {
					t.Fatal(err)
				}
				if err := entry.WithAddress(func(address uintptr) error { return s.Define("optional_event", address) }); err != nil {
					t.Fatal(err)
				}
				getter, err := external.Resolve("total")
				if err != nil {
					t.Fatal(err)
				}
				if err := getter.WithAddress(func(address uintptr) error { return s.Define("total", address) }); err != nil {
					t.Fatal(err)
				}
			}
			roots := []string{"weak_call"}
			if tc.name == "explicit_archive_root" {
				roots = append(roots, "optional_event")
			}
			if err := s.Link(roots...); err != nil {
				t.Fatal(err)
			}
			call(t, s, "weak_call", 20, 22, 42)
			if tc.provided {
				call(t, s, "total", 0, 0, 42)
			}
			call(t, s, "weak_call", -1, 22, 21)
			if tc.provided {
				call(t, s, "total", 0, 0, 63)
			}
			if len(s.image.objects) != tc.objects {
				t.Fatalf("unexpected weak archive extraction: %d", len(s.image.objects))
			}
		})
	}
}

func TestNativeARM64ELFWeakRelativeData(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" {
		t.Skip("AArch64 ELF execution requires Linux arm64")
	}
	needNative(t)
	dir := t.TempDir()
	consumer := compile(t, "testdata/elf_arm_weak_refs.s", filepath.Join(dir, "refs.o"))
	provider := compile(t, "testdata/elf_arm_weak_provider.c", filepath.Join(dir, "provider.o"))
	for _, inputs := range [][]string{{consumer}, {consumer, provider}, {provider, consumer}} {
		s := New(Options{})
		defer s.Close()
		load(t, s, inputs...)
		entry, err := s.Resolve("weak_offsets")
		if err != nil {
			t.Fatal(err)
		}
		var target uintptr
		if len(inputs) == 2 {
			value, err := s.Resolve("weak_value")
			if err != nil {
				t.Fatal(err)
			}
			if err := value.WithAddress(func(p uintptr) error { target = p; return nil }); err != nil {
				t.Fatal(err)
			}
		}
		err = entry.WithAddress(func(p uintptr) error {
			offset := p - s.image.base
			b := s.image.mem[offset : offset+22]
			if target == 0 {
				if le.Uint16(b) != 42 || le.Uint32(b[2:]) != 43 || le.Uint64(b[6:]) != 44 || le.Uint64(b[14:]) != 0 {
					t.Fatalf("unresolved weak relative/absolute data: %x", b)
				}
			} else if le.Uint16(b) != uint16(int64(target)+42-int64(p)) || le.Uint32(b[2:]) != uint32(int64(target)+43-int64(p+2)) || le.Uint64(b[6:]) != uint64(int64(target)+44-int64(p+6)) || le.Uint64(b[14:]) != uint64(target) {
				t.Fatalf("resolved weak relative/absolute data: %x", b)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
