package dylib

import (
	"math"
	"path/filepath"
	"runtime"
	"testing"
)

func armGOTImage(width int, target uint64) (*image, *object) {
	o := &object{info: Info{Format: "ELF", Arch: "arm64"}, sections: []*section{nil, {size: uint64(width + 1)}}, symbols: []symbol{{section: -1, value: target}}}
	im := &image{base: 0x10000000, mem: make([]byte, 256), got: map[uintptr]uintptr{}, stubs: map[uintptr]uintptr{}, gotStart: 64, gotNext: 64, stubStart: 128, stubNext: 128, pointerSize: 8}
	return im, o
}

func TestARM64ELFGOTOffsets(t *testing.T) {
	for _, tc := range []struct {
		typ, width uint32
		target     uint64
		addend     int64
		want       uint64
	}{
		{307, 8, 0x10000020, 3, math.MaxUint64 - 28},
		{308, 4, 0x10000020, 3, 0xffffffe3},
		{314, 4, 0x10000020, 4, 35},
		{315, 4, 0x22334455, 4, 67},
	} {
		im, o := armGOTImage(int(tc.width), tc.target)
		im.mem[0], im.mem[tc.width+1] = 0xaa, 0xaa
		for i := uint32(1); i <= tc.width; i++ {
			im.mem[i] = 0xcc // RELA must replace the encoded field.
		}
		err := im.relocate(o, relocation{section: 1, offset: 1, typ: tc.typ, addend: tc.addend, pair: -1})
		got := uint64(le.Uint32(im.mem[1:]))
		if tc.width == 8 {
			got = le.Uint64(im.mem[1:])
		}
		if err != nil || got != tc.want || im.mem[0] != 0xaa || im.mem[tc.width+1] != 0xaa {
			t.Fatalf("GOT/PLT data %d: %#x, %v; want %#x", tc.typ, got, err, tc.want)
		}
		if tc.typ == 315 && le.Uint64(im.mem[64:]) != tc.target {
			t.Fatal("GOTPCREL32's field bias changed the slot target")
		}
		if err := im.relocate(o, relocation{section: 1, offset: 2, typ: tc.typ, pair: -1}); err == nil {
			t.Fatal("GOT data write beyond section accepted")
		}
	}
	for _, typ := range []uint32{310, 313} {
		im, o := armGOTImage(4, 0x22334455)
		im.gotNext += 8
		le.PutUint32(im.mem, 0xf97ffc45)
		want := uint32(0xf9400445) // slot-GOT=8, not its page offset.
		if typ == 313 {
			want = 0xf9402445 // slot-Page(GOT)=0x48 for this non-page-aligned base.
		}
		if err := im.relocate(o, relocation{section: 1, typ: typ, pair: -1}); err != nil || le.Uint32(im.mem) != want {
			t.Fatalf("GOT load %d: %#x, %v; want %#x", typ, le.Uint32(im.mem), err, want)
		}
	}
	for _, offset := range []int64{0, 4096, 32760, -8, 32768, 1} {
		b := make([]byte, 4)
		le.PutUint32(b, 0xf9400045)
		err := armGOTOffset15(b, uintptr(0x10000000+offset), 0x10000000, 0x10000000)
		valid := offset >= 0 && offset < 32768 && offset&7 == 0
		if valid && (err != nil || le.Uint32(b) != 0xf9400045|uint32(offset>>3)<<10) || !valid && (err == nil || le.Uint32(b) != 0xf9400045) {
			t.Fatalf("15-bit GOT offset %d: %#x, %v", offset, le.Uint32(b), err)
		}
	}
	for _, tc := range []struct {
		ins   uint32
		place uintptr
	}{
		{0xb9400045, 0}, {0x91000045, 0}, {0xf8400045, 0}, {0xf9400045, 1},
		{0xf9800045, 0}, {0xf9c00045, 0}, // PRFM and a reserved integer opcode.
	} {
		b := make([]byte, 4)
		le.PutUint32(b, tc.ins)
		if err := armGOTOffset15(b, 0x10000008, 0x10000000, tc.place); err == nil || le.Uint32(b) != tc.ins {
			t.Fatal("invalid GOT load accepted or changed")
		}
	}
	for _, typ := range []uint32{300, 301, 302, 303, 304, 305, 306, 310, 313} {
		im, o := armGOTImage(4, 0x22334455)
		if err := im.relocate(o, relocation{section: 1, typ: typ, addend: 8, pair: -1}); err == nil || len(im.got) != 0 {
			t.Fatalf("GOT relocation %d accepted a nonzero addend or allocated its slot", typ)
		}
	}
	for _, value := range []int64{math.MinInt32, math.MaxInt32, math.MinInt32 - 1, math.MaxInt32 + 1} {
		im, o := armGOTImage(4, 0)
		im.mem[1] = 0xcc
		err := im.relocate(o, relocation{section: 1, offset: 1, typ: 308, addend: int64(im.gotBase()) + value, pair: -1})
		valid := value >= math.MinInt32 && value <= math.MaxInt32
		if valid && (err != nil || le.Uint32(im.mem[1:]) != uint32(value)) || !valid && (err == nil || im.mem[1] != 0xcc) {
			t.Fatalf("GOTREL32 range %d: %v", value, err)
		}
		im, o = armGOTImage(4, 0x22334455)
		im.mem[1] = 0xcc
		err = im.relocate(o, relocation{section: 1, offset: 1, typ: 315, addend: value - 63, pair: -1})
		if valid && (err != nil || le.Uint32(im.mem[1:]) != uint32(value)) || !valid && (err == nil || im.mem[1] != 0xcc) {
			t.Fatalf("GOTPCREL32 range %d: %v", value, err)
		}
	}
}

func TestARM64ELFGOTWideMoves(t *testing.T) {
	for _, tc := range []struct {
		typ, ins, want uint32
		value          uint64
	}{
		{300, 0x929fffe2, 0xd29579a2, 0xabcd},
		{301, 0xf2800002, 0xf28ef102, 0x1122334455667788},
		{302, 0xd2a00002, 0xd2aaacc2, 0x55667788},
		{303, 0xf2a00002, 0xf2aaacc2, 0x1122334455667788},
		{304, 0xd2c00002, 0xd2c66882, 0x334455667788},
		{305, 0xf2c00002, 0xf2c66882, 0x1122334455667788},
		{306, 0xd2e00002, 0xd2e22442, 0x1122334455667788},
		{300, 0xd2800002, 0x929fffe2, math.MaxUint64 - 0xffff},
		{306, 0xd2e00002, 0x92e00002, math.MaxUint64},
	} {
		b := make([]byte, 4)
		le.PutUint32(b, tc.ins)
		if err := armELFMOVW(b, tc.typ, tc.value, 0x10000000); err != nil || le.Uint32(b) != tc.want {
			t.Fatalf("GOTOFF wide move %d: %#x, %v; want %#x", tc.typ, le.Uint32(b), err, tc.want)
		}
	}
	for _, tc := range []struct {
		typ   uint32
		value int64
	}{
		{300, 1 << 16}, {300, -(1 << 16) - 1},
		{302, 1 << 32}, {302, -(1 << 32) - 1},
		{304, 1 << 48}, {304, -(1 << 48) - 1},
	} {
		b := make([]byte, 4)
		le.PutUint32(b, 0xd2800002)
		if err := armELFMOVW(b, tc.typ, uint64(tc.value), 0); err == nil || le.Uint32(b) != 0xd2800002 {
			t.Fatal("GOTOFF checked move overflow accepted or changed")
		}
	}
	for _, tc := range []struct {
		typ, ins uint32
		place    uintptr
	}{
		{300, 0xf2800002, 0}, {301, 0xd2800002, 0},
		{302, 0xd2a00002, 1}, {303, 0x52c00002, 0},
	} {
		b := make([]byte, 4)
		le.PutUint32(b, tc.ins)
		if err := armELFMOVW(b, tc.typ, 8, tc.place); err == nil || le.Uint32(b) != tc.ins {
			t.Fatal("invalid GOTOFF wide move accepted or changed")
		}
	}
	im, o := armGOTImage(4, 0x22334455)
	im.gotNext += 8
	le.PutUint32(im.mem, 0xd2800002)
	if err := im.relocate(o, relocation{section: 1, typ: 300, pair: -1}); err != nil || le.Uint32(im.mem) != 0xd2800102 || le.Uint64(im.mem[72:]) != 0x22334455 {
		t.Fatalf("GOTOFF did not use the owned slot offset: %#x, %v", le.Uint32(im.mem), err)
	}
}

func TestARM64ELFPLTData(t *testing.T) {
	im, o := armGOTImage(4, 0xf0000000)
	if err := im.relocate(o, relocation{section: 1, offset: 1, typ: 314, addend: 4, pair: -1}); err != nil || le.Uint32(im.mem[1:]) != 131 || le.Uint64(im.mem[136:]) != 0xf0000000 || le.Uint32(im.mem[128:]) != 0x58000050 || le.Uint32(im.mem[132:]) != 0xd61f0200 {
		t.Fatalf("PLT32 field bias or far thunk: %#x, %v", le.Uint32(im.mem[1:]), err)
	}
	for _, typ := range []uint32{314, 315} {
		im, o := armGOTImage(4, 0)
		o.symbols[0] = symbol{name: "missing", global: true, weak: true}
		im.external = func(string) uintptr { return 0 }
		im.resolved = map[string]uintptr{}
		want := uint32(42)
		if typ == 315 {
			want = 105 // Owned zero-target slot-P+42, not S=P.
		}
		if err := im.relocate(o, relocation{section: 1, offset: 1, typ: typ, addend: 42, pair: -1}); err != nil || le.Uint32(im.mem[1:]) != want || le.Uint64(im.mem[64:]) != 0 {
			t.Fatalf("missing weak data %d: %#x, %v", typ, le.Uint32(im.mem[1:]), err)
		}
	}
	im, o = armGOTImage(4, 0)
	o.symbols[0].weak = true // A selected absolute-zero definition stays zero.
	if err := im.relocate(o, relocation{section: 1, offset: 1, typ: 314, pair: -1}); err != nil || le.Uint32(im.mem[1:]) != 0xefffffff {
		t.Fatalf("absolute-zero PLT32 definition became an unresolved weak value: %v", err)
	}
}

func TestARM64ELFOwnedGOTBase(t *testing.T) {
	o := &object{info: Info{Format: "ELF", Arch: "arm64"}, symbols: []symbol{{name: elfGOTBaseName, global: true}}, relocs: []relocation{{symbol: 0, pair: -1}}}
	im := &image{base: 0x10000000, gotStart: 64, objects: []*object{o}, external: func(string) uintptr { t.Fatal("owned GOT base looked up externally"); return 0 }}
	if p, err := im.symbol(o, 0, false); err != nil || p != im.gotBase() {
		t.Fatalf("owned GOT symbol: %#x, %v", p, err)
	}
	if p, err := im.lookup(elfGOTBaseName); err != nil || p != im.gotBase() {
		t.Fatalf("owned GOT lookup: %#x, %v", p, err)
	}
	if refs, err := references(o); err != nil || len(refs) != 0 {
		t.Fatalf("owned base became an archive dependency: %v, %v", refs, err)
	}
	for _, section := range []int{-2, -1, 1} {
		o.symbols[0].section = section
		if err := checkARMELFGOTDefinitions(o); err == nil {
			t.Fatal("reserved GOT definition accepted")
		}
	}
	o.symbols[0].global = false
	if err := checkARMELFGOTDefinitions(o); err != nil {
		t.Fatal("local label with the same spelling was reserved")
	}
	for _, info := range []Info{{Format: "ELF", Arch: "386"}, {Format: "ELF", Arch: "amd64"}, {Format: "COFF", Arch: "arm64"}, {Format: "Mach-O", Arch: "arm64"}} {
		if armELFOwnedGOT(info, elfGOTBaseName) {
			t.Fatal("ARM64 ELF GOT rule applied to another target")
		}
	}
}

func TestARM64ELFGOTMetadata(t *testing.T) {
	path := compile(t, "testdata/elf_arm_got.s", filepath.Join(t.TempDir(), "got.o"), "--target=aarch64-linux-gnu")
	b, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := parse(path, b)
	if err != nil || len(f.info.Unsupported) != 0 {
		t.Fatalf("GOT metadata: %v, %v", f, err)
	}
	counts := make(map[uint32]int)
	for _, r := range f.obj.relocs {
		counts[r.typ]++
	}
	for _, typ := range []uint32{300, 301, 302, 303, 304, 305, 306, 307, 308, 309, 310, 313, 314, 315} {
		if counts[typ] == 0 {
			t.Fatalf("GOT fixture missing relocation %d: %v", typ, counts)
		}
	}
}

func TestNativeARM64ELFGOTOffsets(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" {
		t.Skip("AArch64 ELF execution requires Linux arm64")
	}
	needNative(t)
	dir := t.TempDir()
	consumer := compile(t, "testdata/elf_arm_got.s", filepath.Join(dir, "consumer.o"))
	provider := compile(t, "testdata/elf_arm_got_provider.c", filepath.Join(dir, "provider.o"))
	badBase := compile(t, "testdata/elf_arm_got_redefine.s", filepath.Join(dir, "badbase.o"))
	unused := compile(t, "testdata/missing.c", filepath.Join(dir, "unused.o"))
	archive := filepath.Join(dir, "provider.a")
	command(t, "ar", "rcs", archive, provider, badBase, unused)
	rootArchive := filepath.Join(dir, "root.a")
	command(t, "ar", "rcs", rootArchive, consumer, provider, badBase, unused)
	shared := filepath.Join(dir, "provider.so")
	command(t, compiler(), "-shared", "-fPIC", "testdata/elf_arm_got_provider.c", "-o", shared)
	for _, tc := range []struct {
		name   string
		inputs []string
		count  int
	}{
		{"objects", []string{consumer, provider}, 2},
		{"objects_reversed", []string{provider, consumer}, 2},
		{"archive", []string{consumer, archive}, 2},
		{"archive_roots", []string{rootArchive}, 2},
		{"missing_retry", []string{consumer}, 2},
		{"shared_provider", []string{consumer, shared}, 1},
		{"defined_provider", []string{consumer}, 1},
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
				for _, name := range []string{"datum", "seed_value", "via_table"} {
					entry, err := external.Resolve(name)
					if err != nil {
						t.Fatal(err)
					}
					if err := entry.WithAddress(func(p uintptr) error { return s.Define(name, p) }); err != nil {
						t.Fatal(err)
					}
				}
			}
			roots := []string{"gotpage_eval", elfGOTBaseName}
			if tc.name == "missing_retry" {
				if err := s.Link(roots...); err == nil || s.image != nil {
					t.Fatal("missing GOT provider was published")
				}
				load(t, s, archive)
			}
			if err := s.Link(roots...); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"gotpage_eval", "gotoff_eval", "gotoff_g0_eval", "gotoff_g1_eval", "gotoff_g2_eval", "gotoff_g3_eval", "gotrel64_eval", "gotrel32_eval", "gotpcrel_eval", "plt32_eval"} {
				call(t, s, name, 20, 22, 84)
				call(t, s, name, -1, 22, 63)
			}
			call(t, s, "weak_got_eval", 20, 22, 42)
			if len(s.image.objects) != tc.count {
				t.Fatalf("selected %d objects; want %d", len(s.image.objects), tc.count)
			}
			base, err := s.Lookup(elfGOTBaseName)
			if err != nil || base != s.image.gotBase() {
				t.Fatalf("reserved base lookup: %#x, %v", base, err)
			}
			address, err := s.Lookup("datum")
			slot, ok := s.image.got[address]
			// Provider-first PIC code can allocate datum before the seed.
			// Consumer-first cases must exercise a nonzero offset as well.
			if err != nil || !ok || slot < base || tc.name != "objects_reversed" && slot == base {
				t.Fatalf("unexpected datum GOT slot %#x at base %#x: %v", slot, base, err)
			}
		})
	}
	// Compare the actual small-PIC C producer with its system-linked library.
	// System linkers do not implement every explicit GOTOFF MOVW fixture form.
	system := filepath.Join(dir, "system.so")
	command(t, "gcc", "-shared", "-fpic", "testdata/elf_arm_large.c", "testdata/elf_arm_got_provider.c", "-o", system)
	s := New(Options{})
	defer s.Close()
	load(t, s, system)
	call(t, s, "large_eval", 20, 22, 84)
	// Real GCC small-PIC output uses GOTPAGE_LO15 and the reserved base.
	gcc := filepath.Join(dir, "gcc.o")
	command(t, "gcc", "-O0", "-fpic", "-fno-stack-protector", "-c", "testdata/elf_arm_large.c", "-o", gcc)
	b, err := readFile(gcc)
	if err != nil {
		t.Fatal(err)
	}
	gf, err := parse(gcc, b)
	if err != nil {
		t.Fatal(err)
	}
	gotpage, gotbase := false, false
	for _, r := range gf.obj.relocs {
		gotpage = gotpage || r.typ == 313
		gotbase = gotbase || gf.obj.symbols[r.symbol].name == elfGOTBaseName
	}
	if !gotpage || !gotbase {
		t.Fatal("GCC small-PIC fixture did not exercise GOTPAGE_LO15 and its table base")
	}
	gs := New(Options{})
	defer gs.Close()
	load(t, gs, gcc, provider)
	call(t, gs, "large_eval", 20, 22, 84)
	invalid := New(Options{})
	defer invalid.Close()
	if err := invalid.Load(badBase); err == nil {
		t.Fatal("object redefinition of the GOT base accepted")
	}
	if err := invalid.Define(elfGOTBaseName, 0x1234); err == nil {
		t.Fatal("host redefinition of the GOT base accepted")
	}
}
