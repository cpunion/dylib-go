package dylib

import (
	"debug/elf"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestARM64ELFInstructions(t *testing.T) {
	o := &object{info: Info{Format: "ELF", Arch: "arm64"}}
	const place = uintptr(0x10000000)
	for _, tc := range []struct {
		name          string
		typ           elf.R_AARCH64
		ins, want     uint32
		delta, addend int64
	}{
		{"ADR ignores encoded immediate", elf.R_AARCH64_ADR_PREL_LO21, 0x70ffffe2, 0x10000802, 0x104, -4},
		{"ADR byte offset", elf.R_AARCH64_ADR_PREL_LO21, 0x10000002, 0x30000002, 0, 1},
		{"ADR negative", elf.R_AARCH64_ADR_PREL_LO21, 0x10000002, 0x70ffffe2, 0, -1},
		{"ADR maximum", elf.R_AARCH64_ADR_PREL_LO21, 0x10000000, 0x707fffe0, (1 << 20) - 1, 0},
		{"ADR minimum", elf.R_AARCH64_ADR_PREL_LO21, 0x10000000, 0x10800000, -(1 << 20), 0},
		{"LDR ignores encoded immediate", elf.R_AARCH64_LD_PREL_LO19, 0x18ffffe2, 0x18000802, 0x104, -4},
		{"LDR negative", elf.R_AARCH64_LD_PREL_LO19, 0x18000002, 0x18fff802, -0x100, 0},
		{"LDR maximum", elf.R_AARCH64_LD_PREL_LO19, 0x18000000, 0x187fffe0, (1 << 20) - 4, 0},
		{"LDR minimum", elf.R_AARCH64_LD_PREL_LO19, 0x18000000, 0x18800000, -(1 << 20), 0},
		{"LDRSW", elf.R_AARCH64_LD_PREL_LO19, 0x98000002, 0x98000802, 0x100, 0},
		{"SIMD LDR", elf.R_AARCH64_LD_PREL_LO19, 0x9c000002, 0x9c000802, 0x100, 0},
		{"PRFM", elf.R_AARCH64_LD_PREL_LO19, 0xd8000000, 0xd8000800, 0x100, 0},
		{"B.cond ignores encoded immediate", elf.R_AARCH64_CONDBR19, 0x54ffffeb, 0x5400080b, 0x104, -4},
		{"CBZ", elf.R_AARCH64_CONDBR19, 0x34000000, 0x34000800, 0x100, 0},
		{"CBNZ X", elf.R_AARCH64_CONDBR19, 0xb5000001, 0xb5000801, 0x100, 0},
		{"TBZ", elf.R_AARCH64_TSTBR14, 0x36000000, 0x36000800, 0x100, 0},
		{"TBNZ preserves high bit index", elf.R_AARCH64_TSTBR14, 0xb7f80001, 0xb7f80801, 0x100, 0},
		{"TBZ maximum", elf.R_AARCH64_TSTBR14, 0x36000000, 0x3603ffe0, (1 << 15) - 4, 0},
		{"TBZ minimum", elf.R_AARCH64_TSTBR14, 0x36000000, 0x36040000, -(1 << 15), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := make([]byte, 4)
			le.PutUint32(b, tc.ins)
			err := (&image{}).relocELF(o, relocation{typ: uint32(tc.typ), addend: tc.addend}, b, uintptr(int64(place)+tc.delta), place)
			if err != nil || le.Uint32(b) != tc.want {
				t.Fatalf("got %#x, %v; want %#x", le.Uint32(b), err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		typ   elf.R_AARCH64
		ins   uint32
		delta int64
	}{
		{elf.R_AARCH64_ADR_PREL_LO21, 0x90000000, 0}, // ADRP is not ADR.
		{elf.R_AARCH64_ADR_PREL_LO21, 0x10000000, 1 << 20},
		{elf.R_AARCH64_ADR_PREL_LO21, 0x10000000, -(1 << 20) - 1},
		{elf.R_AARCH64_LD_PREL_LO19, 0xf9400000, 0}, // Base-register LDR.
		{elf.R_AARCH64_LD_PREL_LO19, 0xdc000000, 0}, // Reserved SIMD literal opcode.
		{elf.R_AARCH64_LD_PREL_LO19, 0x18000000, 1},
		{elf.R_AARCH64_LD_PREL_LO19, 0x18000000, 1 << 20},
		{elf.R_AARCH64_LD_PREL_LO19, 0x18000000, -(1 << 20) - 4},
		{elf.R_AARCH64_CONDBR19, 0x14000000, 0}, // Unconditional branch.
		{elf.R_AARCH64_CONDBR19, 0x54000000, 1},
		{elf.R_AARCH64_CONDBR19, 0x54000000, 1 << 20},
		{elf.R_AARCH64_TSTBR14, 0x34000000, 0}, // CBZ is not TBZ.
		{elf.R_AARCH64_TSTBR14, 0x36000000, 1 << 15},
		{elf.R_AARCH64_TSTBR14, 0x36000000, -(1 << 15) - 4},
	} {
		b := make([]byte, 4)
		le.PutUint32(b, tc.ins)
		if err := (&image{}).relocELF(o, relocation{typ: uint32(tc.typ)}, b, uintptr(int64(place)+tc.delta), place); err == nil || le.Uint32(b) != tc.ins {
			t.Fatalf("invalid instruction changed or accepted: %+v, %v", tc, err)
		}
	}
	o.sections, o.symbols = []*section{nil, {size: 4}}, []symbol{{section: -1, value: uint64(place)}}
	im := &image{base: place, mem: make([]byte, 4)}
	if err := im.relocate(o, relocation{section: 1, offset: 1, typ: 274, pair: -1}); err == nil {
		t.Fatal("accepted instruction write outside section")
	}
	for _, ins := range []uint32{0x10000000, 0x18000000, 0x54000000} {
		b := make([]byte, 4)
		le.PutUint32(b, ins)
		for _, typ := range []uint32{273, 274, 280} {
			if err := (&image{}).relocELF(o, relocation{typ: typ}, b, place+1, place+1); err == nil {
				t.Fatal("accepted misaligned instruction place")
			}
		}
	}
}

func TestARM64ELFGOTAndUncheckedPage(t *testing.T) {
	o := &object{info: Info{Format: "ELF", Arch: "arm64"}}
	im := &image{base: 0x10000000, mem: make([]byte, 128), gotStart: 64, gotNext: 64, stubStart: 128, pointerSize: 8, got: map[uintptr]uintptr{}}
	b := make([]byte, 4)
	le.PutUint32(b, 0x58ffffe1)
	if err := im.relocELF(o, relocation{typ: 309}, b, 0x12345678, im.base); err != nil || le.Uint32(b) != 0x58000201 || le.Uint64(im.mem[64:]) != 0x12345678 {
		t.Fatalf("GOT literal: %#x, %v", le.Uint32(b), err)
	}
	for _, typ := range []uint32{309, 311, 312} {
		for _, addend := range []int64{-8, 8} {
			ins := map[uint32]uint32{309: 0x58000000, 311: 0x90000000, 312: 0xf9400000}[typ]
			le.PutUint32(b, ins)
			if err := im.relocELF(o, relocation{typ: typ, addend: addend}, b, 0x87654321, im.base); err == nil || le.Uint32(b) != ins || im.gotNext != 72 {
				t.Fatalf("nonzero GOT addend accepted or changed image: %d, %v", typ, err)
			}
		}
	}
	for _, ins := range []uint32{0x18000000, 0x5c000000, 0xf9400000} {
		le.PutUint32(b, ins)
		if err := im.relocELF(o, relocation{typ: 309}, b, 0x87654321, im.base); err == nil || im.gotNext != 72 {
			t.Fatal("GOT literal accepted a non-pointer load")
		}
	}
	if uint64(^uintptr(0)) > 0xffffffff {
		far := uint64(im.base) + 1<<32
		le.PutUint32(b, 0x90000002)
		if err := im.relocELF(o, relocation{typ: 275}, b, uintptr(far), im.base); err == nil {
			t.Fatal("checked ADRP accepted overflow")
		}
		if err := im.relocELF(o, relocation{typ: 276}, b, uintptr(far), im.base); err != nil || le.Uint32(b) != 0x90800002 {
			t.Fatalf("unchecked ADRP: %#x, %v", le.Uint32(b), err)
		}
	}
}

func TestARM64ELFInstructionMetadata(t *testing.T) {
	path := compile(t, "testdata/elf_arm_instructions.s", filepath.Join(t.TempDir(), "instructions.o"), "--target=aarch64-linux-gnu")
	b, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := parse(path, b)
	if err != nil || len(f.info.Unsupported) != 0 {
		t.Fatalf("instruction metadata: %v, %v", f, err)
	}
	counts := make(map[uint32]int)
	for _, r := range f.obj.relocs {
		counts[r.typ]++
	}
	for typ, want := range map[uint32]int{273: 1, 274: 1, 279: 2, 280: 3, 309: 1} {
		if counts[typ] != want {
			t.Fatalf("relocation counts = %v; type %d want %d", counts, typ, want)
		}
	}
}

func TestNativeARM64ELFInstructions(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" {
		t.Skip("AArch64 ELF execution requires Linux arm64")
	}
	needNative(t)
	dir := t.TempDir()
	consumer := compile(t, "testdata/elf_arm_instructions.s", filepath.Join(dir, "consumer.o"))
	provider := compile(t, "testdata/elf_arm_provider.c", filepath.Join(dir, "provider.o"))
	unused := compile(t, "testdata/missing.c", filepath.Join(dir, "unused.o"))
	archive := filepath.Join(dir, "provider.a")
	command(t, "ar", "rcs", archive, provider, unused)
	rootArchive := filepath.Join(dir, "root.a")
	command(t, "ar", "rcs", rootArchive, consumer, provider, unused)
	for _, tc := range []struct {
		name   string
		inputs []string
	}{
		{"objects", []string{consumer, provider}},
		{"objects_reversed", []string{provider, consumer}},
		{"archive", []string{consumer, archive}},
		{"archive_root", []string{rootArchive}},
		{"missing_definition_retry", []string{consumer}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(Options{})
			defer s.Close()
			load(t, s, tc.inputs...)
			if tc.name == "missing_definition_retry" {
				if err := s.Link("address_eval"); err == nil || !strings.Contains(err.Error(), "unresolved") || s.image != nil {
					t.Fatalf("missing dependency published: %v", err)
				}
				load(t, s, provider)
			}
			if err := s.Link("address_eval"); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"address_eval", "literal_eval", "got_eval"} {
				call(t, s, name, 20, 22, 84)
				call(t, s, name, -1, 22, 63)
			}
			for _, test := range []struct {
				name       string
				a, b, want int32
			}{
				{"cond_eval", 20, 22, 84}, {"cond_eval", 25, 22, 3},
				{"zero_eval", 0, 22, 64}, {"zero_eval", 20, 22, 42},
				{"nonzero_eval", 0, 22, 22}, {"nonzero_eval", 20, 22, 84},
				{"bit_eval", 20, 22, 84}, {"bit_eval", 25, 22, 3},
				{"bit_set_eval", 20, 22, -2}, {"bit_set_eval", 25, 22, 89},
			} {
				call(t, s, test.name, test.a, test.b, test.want)
			}
			if len(s.image.objects) != 2 {
				t.Fatalf("unused archive member extracted: %d", len(s.image.objects))
			}
		})
	}
}
