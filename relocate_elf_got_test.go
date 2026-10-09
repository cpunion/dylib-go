package dylib

import (
	"math"
	"path/filepath"
	"runtime"
	"testing"
)

func TestELFOwnedGOTBase(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64", "386"} {
		o := &object{info: Info{Format: "ELF", Arch: arch}, symbols: []symbol{{name: elfGOTBaseName, global: true}}, relocs: []relocation{{typ: 1, symbol: 0, pair: -1}}}
		im := &image{base: 0x10000000, gotStart: 64, objects: []*object{o}, external: func(string) uintptr { t.Fatal("owned GOT base looked up externally"); return 0 }}
		if p, err := im.symbol(o, 0, false); err != nil || p != im.gotBase() {
			t.Fatalf("%s GOT symbol: %#x, %v", arch, p, err)
		}
		if p, err := im.lookup(elfGOTBaseName); err != nil || p != im.gotBase() {
			t.Fatalf("%s GOT lookup: %#x, %v", arch, p, err)
		}
		if refs, err := references(o); err != nil || len(refs) != 0 {
			t.Fatalf("%s owned base became an archive dependency: %v, %v", arch, refs, err)
		}
		for _, section := range []int{-2, -1, 1} {
			o.symbols[0].section = section
			if err := checkELFGOTDefinitions(o); err == nil {
				t.Fatalf("%s reserved GOT definition accepted", arch)
			}
		}
		o.symbols[0].global = false
		if err := checkELFGOTDefinitions(o); err != nil {
			t.Fatal("local label with the same spelling was reserved")
		}
	}
	for _, info := range []Info{{Format: "ELF", Arch: "riscv64"}, {Format: "COFF", Arch: "amd64"}, {Format: "Mach-O", Arch: "arm64"}} {
		if elfOwnedGOT(info, elfGOTBaseName) {
			t.Fatal("owned ELF GOT rule applied to another target")
		}
	}
}

func x64GOTImage(width int, target uint64) (*image, *object) {
	im, o := armGOTImage(width, target)
	o.info.Arch = "amd64"
	return im, o
}

func TestAMD64ELFGOTOffsets(t *testing.T) {
	for _, tc := range []struct {
		typ, width uint32
		target     uint64
		addend     int64
		want       uint64
	}{
		{3, 4, 0x22334455, 4, 12},
		{25, 8, 0x10000020, 3, math.MaxUint64 - 28},
		{25, 8, 0xf0000000, 4, 0xdfffffc4},
		{26, 4, 0, -4, 59},
		{27, 8, 0x22334455, -4, 4},
		{28, 8, 0x22334455, 4, 75},
		{29, 8, 0, 4, 67},
		{30, 8, 0x22334455, 4, 12},
		{31, 8, 0x10000020, 4, math.MaxUint64 - 27},
		{31, 8, 0xf0000000, 4, 0xdfffffc4},
	} {
		im, o := x64GOTImage(int(tc.width), tc.target)
		im.gotNext += 8 // Distinguish a slot offset from the table base.
		im.mem[0], im.mem[tc.width+1] = 0xaa, 0xaa
		for i := uint32(1); i <= tc.width; i++ {
			im.mem[i] = 0xcc
		}
		r := relocation{section: 1, offset: 1, typ: tc.typ, addend: tc.addend, pair: -1}
		if err := im.relocate(o, r); err != nil {
			t.Fatalf("GOT relocation %d: %v", tc.typ, err)
		}
		got := uint64(le.Uint32(im.mem[1:]))
		if tc.width == 8 {
			got = le.Uint64(im.mem[1:])
		}
		if got != tc.want || im.mem[0] != 0xaa || im.mem[tc.width+1] != 0xaa {
			t.Fatalf("GOT relocation %d: %#x; want %#x, with unchanged guard bytes", tc.typ, got, tc.want)
		}
		if tc.typ == 31 && len(im.stubs) != 0 {
			t.Fatal("full-width PLTOFF64 allocated a branch thunk")
		}
		if tc.typ == 3 || tc.typ == 27 || tc.typ == 28 || tc.typ == 30 {
			if le.Uint64(im.mem[72:]) != tc.target {
				t.Fatal("GOT field bias changed the slot's target")
			}
			if err := im.relocate(o, r); err != nil || len(im.got) != 1 || im.gotNext != 80 {
				t.Fatal("repeated GOT reference did not reuse its slot")
			}
		}
		r.offset++
		if err := im.relocate(o, r); err == nil {
			t.Fatal("GOT write beyond section accepted")
		}
	}
	for _, typ := range []uint32{3, 26} {
		for _, value := range []int64{math.MinInt32, math.MaxInt32, math.MinInt32 - 1, math.MaxInt32 + 1, math.MinInt64, math.MaxInt64} {
			im, o := x64GOTImage(4, 0x22334455)
			addend := value
			if typ == 26 {
				// Use P=GOT so the extrema remain representable as addends.
				o.sections[1].offset = im.gotStart
			}
			offset := int(o.sections[1].offset)
			im.mem[offset] = 0xcc
			err := im.relocate(o, relocation{section: 1, typ: typ, addend: addend, pair: -1})
			valid := value >= math.MinInt32 && value <= math.MaxInt32
			if valid && (err != nil || le.Uint32(im.mem[offset:]) != uint32(value)) || !valid && (err == nil || im.mem[offset] != 0xcc) {
				t.Fatalf("GOT relocation %d range %d: %v", typ, value, err)
			}
		}
	}
	// Eight-byte values retain all bits on 386 test hosts as well.
	for _, typ := range []uint32{27, 28, 29} {
		im, o := x64GOTImage(8, 0x22334455)
		o.sections[1].offset = im.gotStart
		if err := im.relocate(o, relocation{section: 1, typ: typ, addend: math.MinInt64, pair: -1}); err != nil || le.Uint64(im.mem[64:]) != uint64(1)<<63 {
			t.Fatalf("GOT relocation %d lost high bits: %v", typ, err)
		}
	}
}

func TestELFGOTBaseOnlyReferences(t *testing.T) {
	for _, tc := range []struct {
		arch     string
		typ      uint32
		width    int
		implicit bool
		want     uint64
	}{
		{"amd64", 26, 4, false, 68}, {"amd64", 29, 8, false, 68},
		{"386", 10, 4, false, 68}, {"386", 10, 4, true, 75},
	} {
		for _, index := range []int{0, 1} {
			im, o := x64GOTImage(tc.width, 0)
			o.info.Arch = tc.arch
			o.symbols = []symbol{{}, {name: "ignored", global: true}}
			im.external = func(string) uintptr { t.Fatal("GOTPC resolved an ignored symbol"); return 0 }
			le.PutUint32(im.mem, 7)
			r := relocation{section: 1, typ: tc.typ, symbol: index, addend: 4, implicit: tc.implicit, pair: -1}
			o.relocs = []relocation{r}
			if err := im.relocate(o, r); err != nil {
				t.Fatal(err)
			}
			got := uint64(le.Uint32(im.mem))
			if tc.width == 8 {
				got = le.Uint64(im.mem)
			}
			if got != tc.want || len(im.got) != 0 {
				t.Fatalf("base-only %s/%d: %#x, want %#x", tc.arch, tc.typ, got, tc.want)
			}
			if refs, err := references(o); err != nil || len(refs) != 0 {
				t.Fatalf("GOTPC created an archive dependency: %v, %v", refs, err)
			}
		}
	}
	for _, info := range []Info{{Format: "Mach-O", Arch: "amd64"}, {Format: "COFF", Arch: "386"}, {Format: "ELF", Arch: "arm64"}} {
		if elfGOTBaseRelocation(&object{info: info}, 26) || elfGOTBaseRelocation(&object{info: info}, 10) {
			t.Fatal("GOTPC symbol bypass applied to another target")
		}
	}
}

func TestAMD64ELFGOTWeakTargets(t *testing.T) {
	for _, typ := range []uint32{3, 25, 27, 28, 30, 31} {
		im, o := x64GOTImage(8, 0)
		o.symbols[0] = symbol{name: "missing", global: true, weak: true}
		im.external = func(string) uintptr { return 0 }
		im.resolved = map[string]uintptr{}
		if err := im.relocate(o, relocation{section: 1, typ: typ, addend: 4, pair: -1}); err != nil {
			t.Fatal(err)
		}
		want := uint64(4)
		switch typ {
		case 25, 31:
			want = 4 - uint64(im.gotBase())
		case 28:
			want = 68
		}
		got := le.Uint64(im.mem)
		if typ == 3 {
			got = uint64(le.Uint32(im.mem))
		}
		if got != want || le.Uint64(im.mem[64:]) != 0 || len(im.resolved) != 0 {
			t.Fatalf("missing weak relocation %d: %#x, want %#x", typ, got, want)
		}
	}
}

func TestAMD64ELFGOTMetadata(t *testing.T) {
	dir := t.TempDir()
	asm := compile(t, "testdata/elf_x64_got.s", filepath.Join(dir, "got.o"), "--target=x86_64-linux-gnu")
	c := compile(t, "testdata/elf_x64_large.c", filepath.Join(dir, "large.o"), "--target=x86_64-linux-gnu", "-mcmodel=large")
	for path, types := range map[string][]uint32{asm: {3, 25, 26, 27, 28, 29, 30, 31}, c: {25, 27, 29}} {
		b, err := readFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parse(path, b)
		if err != nil || len(f.info.Unsupported) != 0 {
			t.Fatalf("large-model metadata: %v, %v", f, err)
		}
		counts := make(map[uint32]int)
		for _, r := range f.obj.relocs {
			counts[r.typ]++
		}
		for _, typ := range types {
			if counts[typ] == 0 {
				t.Fatalf("%s lacks relocation %d: %v", path, typ, counts)
			}
		}
	}
}

func TestNativeAMD64ELFGOTOffsets(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("x86-64 ELF execution requires Linux amd64")
	}
	needNative(t)
	dir := t.TempDir()
	consumer := compile(t, "testdata/elf_x64_got.s", filepath.Join(dir, "consumer.o"))
	provider := compile(t, "testdata/elf_got_provider.c", filepath.Join(dir, "provider.o"))
	badBase := compile(t, "testdata/elf_got_redefine.s", filepath.Join(dir, "badbase.o"))
	ignored := compile(t, "testdata/elf_got_ignored.s", filepath.Join(dir, "ignored.o"))
	unused := compile(t, "testdata/missing.c", filepath.Join(dir, "unused.o"))
	archive := filepath.Join(dir, "provider.a")
	command(t, "ar", "rcs", archive, provider, badBase, ignored, unused)
	rootArchive := filepath.Join(dir, "root.a")
	command(t, "ar", "rcs", rootArchive, consumer, provider, badBase, ignored, unused)
	shared := filepath.Join(dir, "provider.so")
	command(t, compiler(), "-shared", "-fPIC", "testdata/elf_got_provider.c", "-o", shared)
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
			roots := []string{"got32_eval", elfGOTBaseName}
			if tc.name == "missing_retry" {
				if err := s.Link(roots...); err == nil || s.image != nil {
					t.Fatal("missing GOT provider was published")
				}
				load(t, s, archive)
			}
			if err := s.Link(roots...); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"got32_eval", "got64_eval", "gotoff64_eval", "gotpcrel64_eval", "gotpc64_eval", "gotplt64_eval", "pltoff64_eval"} {
				call(t, s, name, 20, 22, 84)
				call(t, s, name, -1, 22, 63)
			}
			call(t, s, "weak_got_eval", 20, 22, 42)
			if len(s.image.objects) != tc.count {
				t.Fatalf("selected %d objects; want %d", len(s.image.objects), tc.count)
			}
			base, err := s.Lookup(elfGOTBaseName)
			if err != nil || base != s.image.gotBase() {
				t.Fatalf("owned GOT lookup: %#x, %v", base, err)
			}
			for name, width := range map[string]int{"base_null": 4, "base_ignored": 8} {
				p, err := s.Lookup(name)
				if err != nil {
					t.Fatal(err)
				}
				offset := p - s.image.base
				if p < s.image.base || offset > uintptr(len(s.image.mem)) || uintptr(width) > uintptr(len(s.image.mem))-offset {
					t.Fatal("GOTPC data lies outside the owned image")
				}
				b := s.image.mem[offset : offset+uintptr(width)]
				got, want := uint64(le.Uint32(b)), uint64(base)-uint64(p)
				if width == 8 {
					got = le.Uint64(b)
					want += 4
				} else {
					want = uint64(uint32(want))
				}
				if got != want {
					t.Fatalf("%s: %#x, want %#x", name, got, want)
				}
			}
		})
	}
	// Check actual GCC and Clang large-PIC output against OS-linked libraries.
	for _, cc := range []string{"gcc", compiler()} {
		t.Run(cc, func(t *testing.T) {
			path := filepath.Join(dir, filepath.Base(cc)+".o")
			command(t, cc, "-O0", "-fPIC", "-mcmodel=large", "-fno-stack-protector", "-c", "testdata/elf_x64_large.c", "-o", path)
			b, err := readFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(path, b)
			if err != nil {
				t.Fatal(err)
			}
			counts := make(map[uint32]int)
			for _, r := range f.obj.relocs {
				counts[r.typ]++
			}
			for _, typ := range []uint32{25, 27, 29} {
				if counts[typ] == 0 {
					t.Fatalf("%s large-PIC lacks relocation %d", cc, typ)
				}
			}
			if cc == "gcc" && counts[31] == 0 {
				t.Fatal("GCC large-PIC lacks PLTOFF64")
			}
			s := New(Options{})
			defer s.Close()
			load(t, s, path, provider)
			call(t, s, "large_call", 20, 22, 168)
			call(t, s, "large_call", -1, 22, 147)
			library := filepath.Join(dir, filepath.Base(cc)+".so")
			command(t, cc, "-shared", "-fPIC", "-mcmodel=large", "testdata/elf_x64_large.c", "testdata/elf_got_provider.c", "-o", library)
			system := New(Options{})
			defer system.Close()
			load(t, system, library)
			call(t, system, "large_call", 20, 22, 168)
			call(t, system, "large_call", -1, 22, 147)
		})
	}
	invalid := New(Options{})
	defer invalid.Close()
	if err := invalid.Load(badBase); err == nil {
		t.Fatal("object redefinition of GOT base accepted")
	}
	if err := invalid.Define(elfGOTBaseName, 0x1234); err == nil {
		t.Fatal("host redefinition of GOT base accepted")
	}
}

func TestNativeELFGOTBase386(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "386" {
		t.Skip("i386 ELF execution requires Linux 386")
	}
	needNative(t)
	dir := t.TempDir()
	consumer := compile(t, "testdata/elf_arm_large.c", filepath.Join(dir, "consumer.o"))
	provider := compile(t, "testdata/elf_got_provider.c", filepath.Join(dir, "provider.o"))
	badBase := compile(t, "testdata/elf_got_redefine.s", filepath.Join(dir, "badbase.o"))
	b, err := readFile(consumer)
	if err != nil {
		t.Fatal(err)
	}
	f, err := parse(consumer, b)
	if err != nil {
		t.Fatal(err)
	}
	gotpc := false
	for _, r := range f.obj.relocs {
		gotpc = gotpc || r.typ == 10
	}
	if !gotpc {
		t.Fatal("i386 PIC compiler did not emit GOTPC")
	}
	archive := filepath.Join(dir, "provider.a")
	command(t, "ar", "rcs", archive, provider, badBase)
	s := New(Options{})
	defer s.Close()
	load(t, s, consumer, archive)
	if err := s.Link("large_eval", elfGOTBaseName); err != nil {
		t.Fatal(err)
	}
	call(t, s, "large_eval", 20, 22, 84)
	call(t, s, "large_eval", -1, 22, 63)
	base, err := s.Lookup(elfGOTBaseName)
	if err != nil || base != s.image.gotBase() || len(s.image.objects) != 2 {
		t.Fatalf("i386 owned GOT and archive root: %#x, %v", base, err)
	}
	invalid := New(Options{})
	defer invalid.Close()
	if err := invalid.Load(badBase); err == nil {
		t.Fatal("i386 object GOT redefinition accepted")
	}
	if err := invalid.Define(elfGOTBaseName, 0x1234); err == nil {
		t.Fatal("i386 host GOT redefinition accepted")
	}
}
