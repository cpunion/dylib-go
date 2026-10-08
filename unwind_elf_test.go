package dylib

import (
	"encoding/binary"
	"path/filepath"
	"testing"
)

func elfUnwindTestImage() *image {
	mem := make([]byte, 512)
	text := &section{name: ".text", exec: true, size: 64}
	frame := &section{name: ".eh_frame", offset: 128, size: 72, tail: 4}
	copy(mem[128:], []byte{20, 0, 0, 0, 0, 0, 0, 0, 1, 'z', 'R', 0, 1, 0x78, 16, 1, 0x1b, 12, 7, 8, 0x90, 1, 0, 0})
	for i := 0; i < 2; i++ {
		offset := 24 + i*24
		record := mem[128+offset:]
		binary.LittleEndian.PutUint32(record, 20)
		binary.LittleEndian.PutUint32(record[4:], uint32(offset+4))
		binary.LittleEndian.PutUint32(record[8:], uint32(int32(i*32-(128+offset+8))))
		binary.LittleEndian.PutUint32(record[12:], 32)
		// Empty augmentation and no-op CFI padding.
	}
	o := &object{info: Info{Format: "ELF", Arch: "amd64", Bits: 64}, sections: []*section{nil, text, frame}}
	return &image{mem: mem, base: 0x100000, pointerSize: 8, objects: []*object{o}}
}

func TestELFFrameValidation(t *testing.T) {
	im := elfUnwindTestImage()
	frames, err := im.elfFrameSections()
	if err != nil || len(frames) != 1 || frames[0] != im.base+128 {
		t.Fatalf("frame sections: %v, %v", frames, err)
	}
	le := binary.LittleEndian
	cases := []struct {
		name  string
		patch func(*image)
	}{
		{"executable", func(im *image) { im.objects[0].sections[2].exec = true }},
		{"writable", func(im *image) { im.objects[0].sections[2].write = true }},
		{"tail missing", func(im *image) { im.objects[0].sections[2].tail = 0 }},
		{"bad terminator", func(im *image) { im.mem[200] = 1 }},
		{"short length", func(im *image) { im.objects[0].sections[2].size = 1 }},
		{"long record", func(im *image) { le.PutUint32(im.mem[128:], 4096) }},
		{"DWARF64", func(im *image) { le.PutUint32(im.mem[128:], 0xffffffff) }},
		{"CIE version", func(im *image) { im.mem[136] = 4 }},
		{"personality", func(im *image) { im.mem[138] = 'P' }},
		{"unterminated augmentation", func(im *image) {
			for i := 137; i < 152; i++ {
				im.mem[i] = 'z'
			}
		}},
		{"code alignment", func(im *image) { im.mem[140] = 0 }},
		{"augmentation size", func(im *image) { im.mem[143] = 2 }},
		{"indirect encoding", func(im *image) { im.mem[144] = 0x9b }},
		{"variable encoding", func(im *image) { im.mem[144] = 0x11 }},
		{"CFI operand", func(im *image) { im.mem[151] = 5 }},
		{"CFI restore", func(im *image) { im.mem[151] = 11 }},
		{"unknown CFI", func(im *image) { im.mem[151] = 28 }},
		{"FDE CIE", func(im *image) { le.PutUint32(im.mem[156:], 27) }},
		{"FDE external range", func(im *image) { le.PutUint32(im.mem[160:], 4096) }},
		{"FDE empty range", func(im *image) { le.PutUint32(im.mem[164:], 0) }},
		{"FDE crossing range", func(im *image) { le.PutUint32(im.mem[164:], 65) }},
		{"FDE augmentation", func(im *image) { im.mem[168] = 1 }},
		{"early terminator", func(im *image) { le.PutUint32(im.mem[152:], 0) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			im := elfUnwindTestImage()
			tc.patch(im)
			if _, err := im.elfFrameSections(); err == nil {
				t.Fatal("invalid frames accepted")
			}
		})
	}
	// Native-width addressing also works for ELF32. A version 3 CIE uses a
	// ULEB return-address column (same one-byte value in this record).
	im.pointerSize = 4
	im.mem[136] = 3
	if _, err := im.elfFrameSections(); err != nil {
		t.Fatal(err)
	}
	for _, signed := range []bool{false, true} {
		r := frameReader{b: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 2}}
		if _, err := r.leb(signed); err == nil {
			t.Fatal("overflowing LEB128 accepted")
		}
	}
}

func TestELFUnwindProducerMetadata(t *testing.T) {
	for _, triple := range []string{"x86_64-unknown-linux-gnu", "aarch64-unknown-linux-gnu", "i686-unknown-linux-gnu"} {
		path := compile(t, "testdata/unwind_frames.c", filepath.Join(t.TempDir(), "frames.o"), "--target="+triple, "-funwind-tables")
		b, err := readFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parse(path, b)
		if err != nil || len(f.info.Unsupported) != 0 {
			t.Fatalf("producer: %v, %v", f, err)
		}
		var found bool
		for _, s := range f.obj.sections {
			if s != nil && s.name == ".eh_frame" {
				found = s.size > 0 && s.tail == 4
			}
		}
		if !found {
			t.Fatal("compiler .eh_frame missing")
		}
		discardUnwind(f.obj)
		for _, s := range f.obj.sections {
			if s != nil && elfUnwindSection(s.name) {
				t.Fatal("default load retained frame metadata")
			}
		}
		for _, r := range f.obj.relocs {
			if f.obj.sections[r.section] == nil {
				t.Fatal("discarded frame dependency retained")
			}
		}
	}
}
