package dylib

import (
	"bytes"
	"debug/elf"
	"math"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestARM64ELFMOVW(t *testing.T) {
	o := &object{info: Info{Format: "ELF", Arch: "arm64"}}
	for _, tc := range []struct {
		name      string
		typ       uint32
		ins, want uint32
		value     uint64
	}{
		{"unsigned low replaces immediate", 264, 0xd29fffe5, 0xd2822445, 0x7788556633441122},
		{"unsigned group1", 266, 0xf2a00005, 0xf2a66885, 0x7788556633441122},
		{"unsigned group2", 268, 0xf2c00005, 0xf2caacc5, 0x7788556633441122},
		{"unsigned group3", 269, 0xf2e00005, 0xf2eef105, 0x7788556633441122},
		{"unsigned maximum G0", 263, 0xd2800005, 0xd29fffe5, 0xffff},
		{"unsigned maximum G1", 265, 0xd2a00005, 0xd2bfffe5, 0xffffffff},
		{"unsigned maximum G2", 267, 0xd2c00005, 0xd2dfffe5, 0xffffffffffff},
		{"unsigned full high bit", 269, 0xf2e00005, 0xf2f00005, 1 << 63},
		{"unsigned W register", 263, 0x52800005, 0x52800545, 42},
		{"source group differs from destination shift", 265, 0x52800005, 0x52866885, 0x33441122},
		{"signed negative replaces MOVZ", 270, 0xd29fffe5, 0x92800005, math.MaxUint64},
		{"signed positive replaces MOVN", 270, 0x929fffe5, 0xd2800545, 42},
		{"signed minimum G0", 270, 0xd2800005, 0x929fffe5, math.MaxUint64 - 0xffff},
		{"signed maximum G0", 270, 0x92800005, 0xd29fffe5, 0xffff},
		{"signed minimum G1", 271, 0xd2a00005, 0x92bfffe5, math.MaxUint64 - 0xffffffff},
		{"signed maximum G1", 271, 0x92a00005, 0xd2bfffe5, 0xffffffff},
		{"signed minimum G2", 272, 0xd2c00005, 0x92dfffe5, math.MaxUint64 - 0xffffffffffff},
		{"signed maximum G2", 272, 0x92c00005, 0xd2dfffe5, 0xffffffffffff},
		{"relative minimum G0", 287, 0xd2800005, 0x929fffe5, math.MaxUint64 - 0xffff},
		{"relative maximum G1", 289, 0x92a00005, 0xd2bfffe5, 0xffffffff},
		{"relative maximum G2", 291, 0x92c00005, 0xd2dfffe5, 0xffffffffffff},
		{"relative negative G3", 293, 0xd2e00005, 0x92efffe5, 1 << 63},
		{"relative NC low keeps MOVK", 288, 0xf2800005, 0xf29fffc5, math.MaxUint64 - 1},
		{"relative NC group1", 290, 0xf2a00005, 0xf2a66885, 0x7788556633441122},
		{"relative NC group2", 292, 0xf2c00005, 0xf2caacc5, 0x7788556633441122},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := make([]byte, 4)
			le.PutUint32(b, tc.ins)
			// Carry values in RELA's addend so high bits are also tested on 386.
			err := (&image{}).relocELF(o, relocation{typ: tc.typ, addend: int64(tc.value)}, b, 0, 0)
			if err != nil || le.Uint32(b) != tc.want {
				t.Fatalf("got %#x, %v; want %#x", le.Uint32(b), err, tc.want)
			}
		})
	}
	b := make([]byte, 4)
	le.PutUint32(b, 0xd29fffe5)
	if err := (&image{}).relocELF(o, relocation{typ: 287, addend: -18}, b, 0x10000010, 0x10000000); err != nil || le.Uint32(b) != 0x92800025 {
		t.Fatalf("symbol + addend - P: %#x, %v", le.Uint32(b), err)
	}
	for _, tc := range []struct {
		typ   uint32
		ins   uint32
		value int64
		place uintptr
	}{
		{263, 0xd2800005, 1 << 16, 0}, {263, 0xd2800005, -1, 0},
		{265, 0xd2a00005, 1 << 32, 0}, {267, 0xd2c00005, 1 << 48, 0},
		{270, 0xd2800005, -(1 << 16) - 1, 0}, {270, 0xd2800005, 1 << 16, 0},
		{271, 0xd2a00005, -(1 << 32) - 1, 0}, {271, 0xd2a00005, 1 << 32, 0},
		{272, 0xd2c00005, -(1 << 48) - 1, 0}, {272, 0xd2c00005, 1 << 48, 0},
		{287, 0xd2800005, 1 << 16, 0}, {289, 0xd2a00005, -(1 << 32) - 1, 0},
		{291, 0xd2c00005, 1 << 48, 0},
		{264, 0x10000005, 42, 0}, // ADR, not move-wide.
		{264, 0x92800005, 42, 0}, // UABS cannot use MOVN.
		{270, 0xf2800005, 42, 0}, // Signed checking cannot use MOVK.
		{288, 0xd2800005, 42, 0}, // PREL NC must use MOVK.
		{264, 0xb2800005, 42, 0}, // Reserved opcode.
		{264, 0x52c00005, 42, 0}, // W-form cannot shift by 32.
		{264, 0xd2800005, 42, 1}, // Misaligned instruction.
	} {
		le.PutUint32(b, tc.ins)
		if err := (&image{}).relocELF(o, relocation{typ: tc.typ, addend: tc.value}, b, 0, tc.place); err == nil || le.Uint32(b) != tc.ins {
			t.Fatalf("invalid MOVW accepted or changed: %+v, %v", tc, err)
		}
	}
}

func TestARM64ELFNarrowData(t *testing.T) {
	for _, tc := range []struct {
		typ   uint32
		value int64
		want  uint64
		valid bool
	}{
		{259, -32768, 0x8000, true}, {259, 65535, 0xffff, true},
		{259, -32769, 0, false}, {259, 65536, 0, false},
		{262, -32768, 0x8000, true}, {262, 32767, 0x7fff, true},
		{262, -32769, 0, false}, {262, 32768, 0, false},
		{258, math.MinInt32, 0x80000000, true}, {258, math.MaxUint32, 0xffffffff, true},
		{258, math.MinInt32 - 1, 0, false}, {258, math.MaxUint32 + 1, 0, false},
	} {
		width := 2
		if tc.typ == 258 {
			width = 4
		}
		const base = uintptr(0x10000000)
		o := &object{info: Info{Format: "ELF", Arch: "arm64"}, sections: []*section{nil, {size: uint64(width + 1)}}, symbols: []symbol{{section: -1}}}
		im := &image{base: base, mem: bytes.Repeat([]byte{0xaa}, width+2)}
		addend := tc.value
		if tc.typ == 262 {
			addend += int64(base + 1)
		}
		err := im.relocate(o, relocation{section: 1, offset: 1, typ: tc.typ, addend: addend, pair: -1})
		if !tc.valid {
			if err == nil || !bytes.Equal(im.mem, bytes.Repeat([]byte{0xaa}, len(im.mem))) {
				t.Fatalf("overflow accepted or changed bytes: %+v, %v", tc, err)
			}
			continue
		}
		var got uint64
		if width == 2 {
			got = uint64(le.Uint16(im.mem[1:]))
		} else {
			got = uint64(le.Uint32(im.mem[1:]))
		}
		if err != nil || got != tc.want || im.mem[0] != 0xaa || im.mem[len(im.mem)-1] != 0xaa {
			t.Fatalf("narrow data: %+v = %#x, %v", tc, got, err)
		}
		if err := im.relocate(o, relocation{section: 1, offset: 2, typ: tc.typ, pair: -1}); err == nil {
			t.Fatal("accepted data relocation beyond section")
		}
	}
}

func TestARM64ELFNullRelocations(t *testing.T) {
	o := &object{info: Info{Format: "ELF", Arch: "arm64"}, relocs: []relocation{{typ: 0, symbol: -1, pair: -1}, {typ: 256, symbol: 999, pair: 999}}}
	refs, err := references(o)
	if err != nil || len(refs) != 0 {
		t.Fatalf("null relocations added dependencies: %v, %v", refs, err)
	}
	for _, r := range o.relocs {
		if err := (&image{}).relocate(o, r); err != nil {
			t.Fatalf("null relocation inspected ignored fields: %v", err)
		}
	}
	path := compile(t, "testdata/elf_arm_movw.s", filepath.Join(t.TempDir(), "movw.o"), "--target=aarch64-linux-gnu")
	b, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := elf.NewFile(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, typ := range []uint32{0, 256} {
		copy := append([]byte(nil), b...)
		found := false
		for _, sec := range f.Sections {
			if sec.Type != elf.SHT_RELA {
				continue
			}
			for off := sec.Offset; off < sec.Offset+sec.Size; off += 24 {
				if uint32(le.Uint64(copy[off+8:])) == 0 {
					// The old null encoding must also ignore offset/symbol fields.
					le.PutUint64(copy[off:], math.MaxUint64)
					le.PutUint64(copy[off+8:], uint64(math.MaxUint32)<<32|uint64(typ))
					found = true
				}
			}
		}
		parsed, err := parseELF(path, copy)
		if err != nil || !found {
			t.Fatalf("parse null type %d: found %v, %v", typ, found, err)
		}
		refs, err := references(parsed.obj)
		for _, ref := range refs {
			if ref.name == "missing_dependency" {
				t.Fatal("null relocation became an archive dependency")
			}
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestELF386NullRelocation(t *testing.T) {
	b, err := readFile("testdata/elfsizes/linux_386.o")
	if err != nil {
		t.Fatal(err)
	}
	f, err := elf.NewFile(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	count := 0
	for _, sec := range f.Sections {
		if sec.Type != elf.SHT_REL {
			continue
		}
		if count == 0 {
			le.PutUint32(b[sec.Offset:], math.MaxUint32)
			le.PutUint32(b[sec.Offset+4:], 0xffffff00) // Invalid symbol, R_386_NONE.
		}
		count += int(sec.Size / 8)
	}
	parsed, err := parseELF("null386.o", b)
	if err != nil || count == 0 {
		t.Fatalf("parse i386 null relocation: %v", err)
	}
	if len(parsed.obj.relocs) != count-1 {
		t.Fatal("i386 null relocation was retained")
	}
	if _, err := references(parsed.obj); err != nil {
		t.Fatalf("i386 null symbol became a dependency: %v", err)
	}
}

func TestARM64ELFMOVWMetadata(t *testing.T) {
	dir := t.TempDir()
	asm := compile(t, "testdata/elf_arm_movw.s", filepath.Join(dir, "movw.o"), "--target=aarch64-linux-gnu")
	large := compile(t, "testdata/elf_arm_large.c", filepath.Join(dir, "large.o"), "--target=aarch64-linux-gnu", "-mcmodel=large", "-fno-pic", "-fno-pie")
	counts := make(map[uint32]int)
	for _, path := range []string{asm, large} {
		b, err := readFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parse(path, b)
		if err != nil || len(f.info.Unsupported) != 0 {
			t.Fatalf("MOVW metadata: %v, %v", f, err)
		}
		for _, r := range f.obj.relocs {
			counts[r.typ]++
		}
	}
	for _, typ := range []uint32{258, 259, 262, 263, 264, 265, 266, 267, 268, 269, 270, 271, 272, 287, 288, 289, 290, 291, 292, 293} {
		if counts[typ] == 0 {
			t.Fatalf("fixture missing relocation %d: %v", typ, counts)
		}
	}
}

func TestNativeARM64ELFMOVW(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" {
		t.Skip("AArch64 ELF execution requires Linux arm64")
	}
	needNative(t)
	dir := t.TempDir()
	asm := compile(t, "testdata/elf_arm_movw.s", filepath.Join(dir, "movw.o"))
	large := compile(t, "testdata/elf_arm_large.c", filepath.Join(dir, "large.o"), "-mcmodel=large", "-fno-pic", "-fno-pie")
	provider := compile(t, "testdata/elf_arm_provider.c", filepath.Join(dir, "provider.o"))
	constants := compile(t, "testdata/elf_arm_constants.s", filepath.Join(dir, "constants.o"))
	unused := compile(t, "testdata/missing.c", filepath.Join(dir, "unused.o"))
	archive := filepath.Join(dir, "provider.a")
	command(t, "ar", "rcs", archive, provider, constants, unused)
	rootArchive := filepath.Join(dir, "root.a")
	command(t, "ar", "rcs", rootArchive, asm, large, provider, constants, unused)
	for _, tc := range []struct {
		name   string
		inputs []string
	}{
		{"objects", []string{asm, large, provider, constants}},
		{"objects_reversed", []string{constants, provider, large, asm}},
		{"archive", []string{asm, large, archive}},
		{"archive_root", []string{rootArchive}},
		{"missing_definition_retry", []string{asm, large}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(Options{})
			defer s.Close()
			load(t, s, tc.inputs...)
			if tc.name == "missing_definition_retry" {
				if err := s.Link("large_eval", "prel_eval"); err == nil || !strings.Contains(err.Error(), "unresolved") || s.image != nil {
					t.Fatalf("missing dependency published: %v", err)
				}
				load(t, s, provider, constants)
			}
			if err := s.Link("large_eval", "prel_eval"); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"large_eval", "unsigned_eval", "signed_positive_eval", "signed_negative_eval", "signed_mid_eval", "signed_high_eval", "prel_eval", "prel_short_eval", "prel_mid_eval", "prel_high_eval", "narrow_eval"} {
				call(t, s, name, 20, 22, 84)
				call(t, s, name, -1, 22, 63)
			}
			if len(s.image.objects) != 4 {
				t.Fatalf("unused archive dependency selected: %d", len(s.image.objects))
			}
		})
	}
}
