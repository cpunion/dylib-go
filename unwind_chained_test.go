package dylib

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"
)

func chainedUnwindTestImage() *image {
	im := unwindTestImage("amd64")
	le := binary.LittleEndian
	// Two secondary ranges share a primary. The intermediate entry is only
	// embedded in .xdata, rather than independently registered in .pdata.
	copy(im.mem[72:], []byte{0x21, 5, 2, 0, 5, 0x34, 5, 0})
	le.PutUint32(im.mem[80:], 0)
	le.PutUint32(im.mem[84:], 32)
	le.PutUint32(im.mem[88:], 64)
	copy(im.mem[92:], []byte{0x21, 5, 2, 0, 5, 0x64, 6, 0})
	le.PutUint32(im.mem[100:], 16)
	le.PutUint32(im.mem[104:], 32)
	le.PutUint32(im.mem[108:], 72)
	le.PutUint32(im.mem[136:], 92)
	return im
}

func TestWindowsChainedUnwindValidation(t *testing.T) {
	le := binary.LittleEndian
	im := chainedUnwindTestImage()
	if table, err := im.windowsFunctionTable("amd64"); err != nil || len(table) != 24 {
		t.Fatalf("multi-level shared primary: %x, %v", table, err)
	}
	t.Run("empty secondary", func(t *testing.T) {
		im := chainedUnwindTestImage()
		copy(im.mem[92:], []byte{0x21, 0, 0, 0})
		le.PutUint32(im.mem[96:], 16)
		le.PutUint32(im.mem[100:], 32)
		le.PutUint32(im.mem[104:], 72)
		if _, err := im.windowsFunctionTable("amd64"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("odd slot count and far save", func(t *testing.T) {
		im := chainedUnwindTestImage()
		copy(im.mem[92:], []byte{0x21, 5, 3, 0, 5, 0x65, 48, 0, 0, 0, 0, 0})
		le.PutUint32(im.mem[104:], 16)
		le.PutUint32(im.mem[108:], 32)
		le.PutUint32(im.mem[112:], 72)
		if _, err := im.windowsFunctionTable("amd64"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("frame register", func(t *testing.T) {
		im := chainedUnwindTestImage()
		copy(im.mem[64:], []byte{1, 4, 2, 0x25, 4, 3, 4, 0x32})
		im.mem[75], im.mem[95] = 0x25, 0x25
		if _, err := im.windowsFunctionTable("amd64"); err != nil {
			t.Fatal(err)
		}
	})
	for _, tc := range []struct {
		name  string
		patch func(*image)
	}{
		{"direct cycle", func(im *image) { le.PutUint32(im.mem[108:], 92) }},
		{"indirect cycle", func(im *image) { le.PutUint32(im.mem[88:], 92) }},
		{"missing primary", func(im *image) { le.PutUint32(im.mem[88:], 252) }},
		{"unaligned primary", func(im *image) { le.PutUint32(im.mem[88:], 65) }},
		{"executable metadata", func(im *image) { le.PutUint32(im.mem[88:], 0) }},
		{"truncated chain", func(im *image) { im.objects[0].sections[2].size = 47 }},
		{"empty parent", func(im *image) { le.PutUint32(im.mem[104:], 16) }},
		{"reversed parent", func(im *image) { le.PutUint32(im.mem[104:], 15) }},
		{"data parent", func(im *image) { le.PutUint32(im.mem[100:], 64); le.PutUint32(im.mem[104:], 72) }},
		{"oversized parent", func(im *image) { le.PutUint32(im.mem[104:], ^uint32(0)) }},
		{"parent handler", func(im *image) { im.mem[64] = 9 }},
		{"secondary handler", func(im *image) { im.mem[92] |= 8 }},
		{"reserved flags", func(im *image) { im.mem[92] |= 64 }},
		{"frame mismatch", func(im *image) { im.mem[95] = 5 }},
		{"parent frame mismatch", func(im *image) { im.mem[67] = 5 }},
		{"parent prolog range", func(im *image) { im.mem[65] = 33 }},
		{"secondary prolog range", func(im *image) { im.mem[93] = 33 }},
		{"secondary stack allocation", func(im *image) { im.mem[97] = 0x32 }},
		{"secondary push", func(im *image) { im.mem[97] = 0x30 }},
		{"missing save operand", func(im *image) { im.mem[94] = 1 }},
		{"parent invalid opcode", func(im *image) { im.mem[69] = 6 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			im := chainedUnwindTestImage()
			tc.patch(im)
			if _, err := im.windowsFunctionTable("amd64"); err == nil {
				t.Fatal("malformed chain accepted")
			}
		})
	}
}

func TestWindowsDeepChainedUnwind(t *testing.T) {
	const count = 2048
	mem := make([]byte, 32+count*16)
	im := &image{mem: mem, objects: []*object{{sections: []*section{
		{exec: true, size: 32}, {offset: 32, size: count * 16},
	}}}}
	le := binary.LittleEndian
	for i := 0; i < count; i++ {
		p := 32 + i*16
		mem[p] = 0x21
		le.PutUint32(mem[p+8:], 32)
		le.PutUint32(mem[p+12:], uint32(p+16))
	}
	last := 32 + (count-1)*16
	mem[last] = 1
	if err := im.x64UnwindInfo(32, 32); err != nil {
		t.Fatal(err)
	}
	mem[last] = 0x21
	le.PutUint32(mem[last+12:], 32)
	if err := im.x64UnwindInfo(32, 32); err == nil || !strings.Contains(err.Error(), "cyclic") {
		t.Fatalf("deep cycle: %v", err)
	}
}

func TestWindowsChainedUnwindProducerMetadata(t *testing.T) {
	for _, triple := range []string{"x86_64-pc-windows-msvc", "x86_64-w64-windows-gnu"} {
		t.Run(triple, func(t *testing.T) {
			path := compile(t, "testdata/unwind_chained_amd64.S", filepath.Join(t.TempDir(), "chain.o"), "--target="+triple)
			im := chainedUnwindMetadataImage(t, path)
			table, err := im.windowsFunctionTable("amd64")
			if err != nil || len(table) != 48 {
				t.Fatalf("producer table: %x, %v", table, err)
			}
			chains := 0
			for p := 0; p < len(table); p += 12 {
				rva := le.Uint32(table[p+8:])
				if im.mem[rva] == 0x21 {
					chains++
				}
			}
			if chains != 2 {
				t.Fatalf("producer emitted %d chained ranges", chains)
			}
		})
	}
}

func chainedUnwindMetadataImage(t *testing.T, path string) *image {
	t.Helper()
	data, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := parse(path, data)
	if err != nil || len(f.info.Unsupported) != 0 {
		t.Fatal(f, err)
	}
	defs, err := definitions([]*object{f.obj})
	if err != nil {
		t.Fatal(err)
	}
	// Relocate data into ordinary Go storage, without executing call fixups.
	// This exercises genuine assembler output in the CGO_ENABLED=0 jobs.
	im := &image{objects: []*object{f.obj}, defs: defs, page: 16}
	size := uint64(16)
	for _, s := range f.obj.sections {
		if s == nil {
			continue
		}
		size = alignUp(size, 16)
		s.offset = size
		size += s.size
	}
	size, err = im.layoutWindowsUnwindProxies(size)
	if err != nil {
		t.Fatal(err)
	}
	im.mem = make([]byte, size)
	for _, s := range f.obj.sections {
		if s != nil {
			copy(im.mem[s.offset:], s.data)
		}
	}
	for _, r := range f.obj.relocs {
		if f.obj.sections[r.section].exec {
			continue
		}
		if r.typ != 3 && r.typ != 1 {
			t.Fatalf("unexpected data relocation: %+v", r)
		}
		if err := im.relocate(f.obj, r); err != nil {
			t.Fatal(err)
		}
	}
	return im
}

func TestWindowsOverlappingChainedProducer(t *testing.T) {
	for _, triple := range []string{"x86_64-pc-windows-msvc", "x86_64-w64-windows-gnu"} {
		t.Run(triple, func(t *testing.T) {
			path := compile(t, "testdata/unwind_chained_overlap_amd64.S", filepath.Join(t.TempDir(), "overlap.o"), "--target="+triple)
			im := chainedUnwindMetadataImage(t, path)
			table, err := im.windowsFunctionTable("amd64")
			if err != nil || len(table) != 7*12 || im.unwindProxyUsed != 32 {
				t.Fatalf("normalized assembler ranges: %x, proxies=%d, %v", table, im.unwindProxyUsed, err)
			}
			for p := 0; p < len(table); p += 12 {
				begin, end, rva := le.Uint32(table[p:]), le.Uint32(table[p+4:]), le.Uint32(table[p+8:])
				if p != 0 && begin < le.Uint32(table[p-8:]) {
					t.Fatal("normalized ranges overlap")
				}
				if err := im.x64UnwindInfo(rva, uint64(end-begin)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
