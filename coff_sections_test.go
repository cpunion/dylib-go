package dylib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestForeignCOFFSectionRelocations(t *testing.T) {
	for _, target := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "sections.s")
			text := ".data\n.long 0\n.globl target\ntarget:\n.long 7\n.section .rdata,\"dr\"\n.secidx target\n.secrel32 target\n"
			if err := os.WriteFile(src, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			out := src + ".o"
			command(t, compiler(), "--target="+target, "-c", src, "-o", out)
			data, err := readFile(out)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := parse(out, data)
			if err != nil {
				t.Fatal(err)
			}
			o := parsed.obj
			defs, err := definitions([]*object{o})
			if err != nil {
				t.Fatal(err)
			}
			im := &image{base: 0x10000000, objects: []*object{o}, defs: defs, mem: make([]byte, len(o.sections)*32)}
			ordinal, targetOrdinal := uint16(0), uint16(0)
			for i, s := range o.sections {
				if s == nil {
					continue
				}
				ordinal++
				s.offset = uint64(i * 32)
				copy(im.mem[s.offset:], s.data)
				if i == defs["target"].sym().section {
					targetOrdinal = ordinal
				}
			}
			if len(o.relocs) != 2 {
				t.Fatalf("fixture relocations: %+v", o.relocs)
			}
			for _, r := range o.relocs {
				if err := im.relocate(o, r); err != nil {
					t.Fatal(err)
				}
				b := im.mem[o.sections[r.section].offset+r.offset:]
				if r.offset == 0 && le.Uint16(b) != targetOrdinal {
					t.Fatal("wrong section ordinal")
				}
				if r.offset == 2 && le.Uint32(b) != 4 {
					t.Fatal("wrong section-relative offset")
				}
			}
		})
	}
}

func TestARM64COFFSectionImmediateAddends(t *testing.T) {
	for _, tc := range []struct {
		typ, ins uint32
		value    uint64
		expected uint32
	}{
		{9, 0x91001400, 0x1010, 0x91005400},  // low ADD: 0x10 + 5.
		{10, 0x91401400, 0x1fff, 0x91400800}, // high ADD: (0x1fff + 5) >> 12.
		{11, 0xf9400400, 0x1020, 0xf9401400}, // 64-bit LDR: 0x20 + 8.
	} {
		o := &object{info: Info{Format: "COFF", Arch: "arm64"}, sections: []*section{nil, {size: 0x3000}}, symbols: []symbol{{section: 1, value: tc.value}}}
		im := &image{objects: []*object{o}}
		b := make([]byte, 4)
		le.PutUint32(b, tc.ins)
		if err := im.relocCOFFSection(o, relocation{typ: tc.typ, symbol: 0}, b); err != nil || le.Uint32(b) != tc.expected {
			t.Fatalf("%d: got %#x %v, want %#x", tc.typ, le.Uint32(b), err, tc.expected)
		}
	}
}
