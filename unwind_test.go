package dylib

import (
	"encoding/binary"
	"path/filepath"
	"testing"
)

func unwindTestImage(arch string) *image {
	mem := make([]byte, 256)
	text := &section{name: ".text", exec: true, offset: 0, size: 64}
	data := &section{name: ".xdata", offset: 64, size: 64}
	width := uint64(12)
	if arch == "arm64" {
		width = 8
	}
	pdata := &section{name: ".pdata", offset: 128, size: 2 * width}
	o := &object{info: Info{Format: "COFF", Arch: arch, Bits: 64}, sections: []*section{nil, text, data, pdata}}
	im := &image{mem: mem, objects: []*object{o}}
	le := binary.LittleEndian
	// Deliberately reverse function order; the output table must be sorted.
	le.PutUint32(mem[128:], 32)
	le.PutUint32(mem[128+width:], 0)
	if arch == "amd64" {
		le.PutUint32(mem[132:], 64)
		le.PutUint32(mem[136:], 64)
		le.PutUint32(mem[144:], 32)
		le.PutUint32(mem[148:], 64)
		copy(mem[64:], []byte{1, 4, 1, 0, 4, 0x32, 0, 0}) // ALLOC_SMALL.
	} else {
		le.PutUint32(mem[132:], 1|8<<2)       // Packed 32-byte function.
		le.PutUint32(mem[140:], 64)           // Full .xdata for the other function.
		le.PutUint32(mem[64:], 8|1<<21|1<<27) // E=1, one code word.
		le.PutUint32(mem[68:], 0xe4e3e3e3)    // nop/nop/nop/end.
	}
	return im
}

func TestWindowsFunctionTableValidation(t *testing.T) {
	le := binary.LittleEndian
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			im := unwindTestImage(arch)
			table, err := im.windowsFunctionTable(arch)
			width := 12
			if arch == "arm64" {
				width = 8
			}
			if err != nil || len(table) != width*2 || le.Uint32(table) != 0 || le.Uint32(table[width:]) != 32 {
				t.Fatalf("sorted table: %x, %v", table, err)
			}
			table[0] = 1
			if le.Uint32(im.mem[128+width:]) != 0 {
				t.Fatal("table aliases input storage")
			}
			patches := []func(*image){
				func(im *image) { im.objects[0].sections[3].size-- },
				func(im *image) { im.objects[0].sections[3].exec = true },
				func(im *image) { le.PutUint32(im.mem[128:], 65) },
				func(im *image) { le.PutUint32(im.mem[128:], 16) }, // Overlapping ranges.
			}
			if arch == "amd64" {
				patches = append(patches,
					func(im *image) { im.mem[64] = 9 },                 // Exception handler flag.
					func(im *image) { im.mem[64] = 2 },                 // Unsupported version.
					func(im *image) { im.mem[66] = 255 },               // Truncated slots.
					func(im *image) { im.mem[69] = 1 },                 // ALLOC_LARGE operand missing.
					func(im *image) { im.mem[68] = 5 },                 // Offset exceeds prolog.
					func(im *image) { im.mem[69] = 6 },                 // Reserved opcode.
					func(im *image) { le.PutUint32(im.mem[136:], 65) }, // Misalignment.
					func(im *image) { le.PutUint32(im.mem[132:], 32) }, // Empty function.
				)
			} else {
				patches = append(patches,
					func(im *image) { le.PutUint32(im.mem[132:], 3) },
					func(im *image) { le.PutUint32(im.mem[132:], 1) }, // Zero packed length.
					func(im *image) { le.PutUint32(im.mem[132:], 1|8<<2|15<<16) },
					func(im *image) { im.mem[66] |= 4 },  // Version 1.
					func(im *image) { im.mem[66] |= 16 }, // Handler flag.
					func(im *image) { le.PutUint32(im.mem[64:], 8|1<<21|31<<27) },
					func(im *image) { le.PutUint32(im.mem[128:], 33) },
					func(im *image) { le.PutUint32(im.mem[140:], 0) }, // Executable metadata.
				)
			}
			for i, patch := range patches {
				im := unwindTestImage(arch)
				patch(im)
				if _, err := im.windowsFunctionTable(arch); err == nil {
					t.Fatalf("invalid table case %d accepted", i)
				}
			}
		})
	}
	// ARM64 extended header and epilog scope bounds.
	im := unwindTestImage("arm64")
	le.PutUint32(im.mem[64:], 8)
	le.PutUint32(im.mem[68:], 1|1<<16)
	le.PutUint32(im.mem[72:], 7) // Scope at byte 28, code index 0.
	le.PutUint32(im.mem[76:], 0xe4e3e3e3)
	if _, err := im.windowsFunctionTable("arm64"); err != nil {
		t.Fatal(err)
	}
	le.PutUint32(im.mem[72:], 8)
	if _, err := im.windowsFunctionTable("arm64"); err == nil {
		t.Fatal("epilog outside function accepted")
	}
}

func TestCOFFUnwindProducerMetadata(t *testing.T) {
	for _, triple := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc"} {
		path := compile(t, "testdata/unwind_frames.c", filepath.Join(t.TempDir(), "frames.o"), "--target="+triple, "-funwind-tables")
		b, err := readFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parse(path, b)
		if err != nil || len(f.info.Unsupported) != 0 {
			t.Fatalf("unwind producer: %v, %v", f, err)
		}
		var count uint64
		for _, s := range f.obj.sections {
			if s != nil && s.name == ".pdata" {
				count += s.size
			}
		}
		width := uint64(12)
		if f.info.Arch == "arm64" {
			width = 8
		}
		if count < 2*width || count%width != 0 {
			t.Fatalf("compiler unwind records: %d bytes", count)
		}
		discardCOFFUnwind(f.obj)
		for _, r := range f.obj.relocs {
			if f.obj.sections[r.section] == nil {
				t.Fatal("default loading retained discarded metadata relocations")
			}
		}
	}
}
