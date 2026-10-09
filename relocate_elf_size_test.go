package dylib

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestELFSizeRelocationArithmetic(t *testing.T) {
	for _, tc := range []struct {
		name, arch string
		typ        uint32
		size       uint64
		addend     int64
		implicit   bool
		word, want uint64
		fails      bool
	}{
		{"32_positive", "amd64", elfSize32, 37, 5, false, 99, 42, false},
		{"32_negative", "amd64", elfSize32, 37, -5, false, 99, 32, false},
		{"32_unsigned", "amd64", elfSize32, math.MaxUint32, 0, false, 0, math.MaxUint32, false},
		{"32_reduced", "amd64", elfSize32, 1 << 32, -1, false, 0, math.MaxUint32, false},
		{"32_large_addend", "amd64", elfSize32, 0, math.MaxInt64, false, 0, 0, true},
		{"32_positive_overflow", "amd64", elfSize32, math.MaxUint32, 1, false, 0, 0, true},
		{"32_negative_overflow", "amd64", elfSize32, 4, -5, false, 0, 0, true},
		{"32_size_overflow", "amd64", elfSize32, math.MaxUint64, -1, false, 0, 0, true},
		{"32_min_addend", "amd64", elfSize32, 1 << 63, math.MinInt64, false, 0, 0, false},
		{"64_positive", "amd64", elfSize64, 37, 5, false, 99, 42, false},
		{"64_negative", "amd64", elfSize64, 37, -5, false, 99, 32, false},
		{"64_wrap", "amd64", elfSize64, math.MaxUint64, 1, false, 99, 0, false},
		{"386_rel", "386", elf386Size32, 37, 0, true, math.MaxUint32 - 4, 32, false},
		{"386_rela", "386", elf386Size32, 37, 5, false, 99, 42, false},
		{"386_wrap", "386", elf386Size32, 0, -1, false, 0, math.MaxUint32, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &object{info: Info{Format: "ELF", Arch: tc.arch}, sections: []*section{nil, {size: 8}}, symbols: []symbol{{section: -1, value: 0x1234, size: tc.size}}}
			im := &image{mem: make([]byte, 8)}
			le.PutUint64(im.mem, tc.word)
			err := im.relocate(o, relocation{section: 1, typ: tc.typ, symbol: 0, addend: tc.addend, implicit: tc.implicit, pair: -1})
			if (err != nil) != tc.fails {
				t.Fatalf("error = %v; want failure %v", err, tc.fails)
			}
			if tc.fails {
				if le.Uint64(im.mem) != tc.word {
					t.Fatal("failed relocation changed memory")
				}
				return
			}
			got := le.Uint64(im.mem)
			if tc.typ != elfSize64 {
				got = uint64(le.Uint32(im.mem))
			}
			if got != tc.want {
				t.Fatalf("got %d; want %d", got, tc.want)
			}
		})
	}
}

func TestELFSizeDefinitionsAndBounds(t *testing.T) {
	consumer := &object{info: Info{Format: "ELF", Arch: "amd64"}, sections: []*section{nil, {size: 8}}, symbols: []symbol{{}, {name: "payload", global: true}}}
	weak := &object{symbols: []symbol{{name: "payload", global: true, weak: true, section: -1, size: 5}}}
	strong := &object{symbols: []symbol{{name: "payload", global: true, section: -1, size: 37}}}
	defs, err := definitions([]*object{weak, strong})
	if err != nil {
		t.Fatal(err)
	}
	im := &image{defs: defs, mem: make([]byte, 8)}
	if size, err := im.elfSymbolSize(consumer, 1); err != nil || size != 37 {
		t.Fatalf("strong override size = %d, %v", size, err)
	}
	consumer.symbols[1].global = false
	consumer.symbols[1].section, consumer.symbols[1].size = -1, 9
	if size, err := im.elfSymbolSize(consumer, 1); err != nil || size != 9 {
		t.Fatalf("local size = %d, %v", size, err)
	}
	consumer.symbols[1].global = true
	im.defs = nil
	consumer.symbols[1].section, consumer.symbols[1].weak = 0, true
	if size, err := im.elfSymbolSize(consumer, 1); err != nil || size != 0 {
		t.Fatalf("undefined weak size = %d, %v", size, err)
	}
	if size, err := im.elfSymbolSize(consumer, 0); err != nil || size != 0 {
		t.Fatalf("null symbol size = %d, %v", size, err)
	}
	for _, weak := range []bool{false, true} {
		consumer.symbols[1].weak = weak
		im.external = func(string) uintptr { return 0x1000 }
		if _, err := im.elfSymbolSize(consumer, 1); err == nil || !strings.Contains(err.Error(), "size unavailable") {
			t.Fatalf("guessed external size: %v", err)
		}
	}
	consumer.symbols[1].section = -2
	im.common = map[string]commonAllocation{"payload": {offset: 0x1000, size: 29}}
	if size, err := im.elfSymbolSize(consumer, 1); err != nil || size != 29 {
		t.Fatalf("merged common size = %d, %v", size, err)
	}
	for _, r := range []relocation{
		{section: 1, offset: 1, typ: elfSize64, symbol: 1, pair: -1},
		{section: 1, offset: math.MaxUint64, typ: elfSize32, symbol: 1, pair: -1},
		{section: 1, typ: elfSize32, symbol: 99, pair: -1},
	} {
		if err := im.relocate(consumer, r); err == nil {
			t.Fatalf("accepted malformed relocation %+v", r)
		}
	}
	consumer.symbols[1].section = 2
	if _, err := im.elfSymbolSize(consumer, 1); err == nil {
		t.Fatal("accepted unloaded definition")
	}
}

func elfSizeFixture(t *testing.T, arch string) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "sizes.o")
	if arch == "386" {
		if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" && runtime.GOARCH != "386" {
			t.Skip("i386 SIZE fixture requires GNU x86 as on Linux")
		}
		preprocessed := filepath.Join(dir, "sizes.s")
		command(t, compiler(), "--target=i686-linux-gnu", "-E", "-P", "-x", "assembler-with-cpp", "testdata/elf_sizes.S", "-o", preprocessed)
		command(t, "as", "--32", preprocessed, "-o", out)
	} else {
		command(t, compiler(), "--target=x86_64-linux-gnu", "-c", "testdata/elf_sizes.S", "-o", out)
	}
	return out
}

func TestELFSizeObjectMetadata(t *testing.T) {
	for _, arch := range []string{"amd64", "386"} {
		t.Run(arch, func(t *testing.T) {
			path := elfSizeFixture(t, arch)
			b, err := readFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(path, b)
			if err != nil || len(f.info.Unsupported) != 0 {
				t.Fatalf("SIZE metadata: %v, %v", f, err)
			}
			counts := make(map[uint32]int)
			local := false
			for _, r := range f.obj.relocs {
				counts[r.typ]++
				if s := f.obj.symbols[r.symbol]; s.name == "local_payload" {
					local = !s.global && s.size == 9
				}
				if r.implicit != (arch == "386") {
					t.Fatal("wrong REL/RELA addend form")
				}
			}
			if !local || arch == "amd64" && (counts[elfSize32] != 6 || counts[elfSize64] != 1) || arch == "386" && counts[elf386Size32] != 6 {
				t.Fatalf("SIZE relocations: counts=%v local=%v", counts, local)
			}
		})
	}
}

func TestNativeELFSizeRelocations(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" && runtime.GOARCH != "386" {
		t.Skip("ELF SIZE execution requires Linux x86")
	}
	needNative(t)
	dir := t.TempDir()
	sizes := elfSizeFixture(t, runtime.GOARCH)
	provider := compile(t, "testdata/elf_size_provider.c", filepath.Join(dir, "provider.o"), "-fcommon")
	consumer := compile(t, "testdata/elf_size_consumer.c", filepath.Join(dir, "consumer.o"))
	weakSource := filepath.Join(dir, "weak.c")
	if err := os.WriteFile(weakSource, []byte("__attribute__((weak)) char payload[5];\n"), 0600); err != nil {
		t.Fatal(err)
	}
	weak := compile(t, weakSource, weakSource+".o")
	unused := compile(t, "testdata/missing.c", filepath.Join(dir, "unused.o"))
	archive := filepath.Join(dir, "sizes.a")
	command(t, "ar", "rcs", archive, sizes, provider, unused)
	rootArchive := filepath.Join(dir, "root.a")
	command(t, "ar", "rcs", rootArchive, sizes, provider, consumer, unused)
	shared := filepath.Join(dir, "sizes.so")
	// Compare against the system linker's static SIZE values. Avoid leaving an
	// undefined weak SIZE relocation for the dynamic loader to dereference.
	command(t, compiler(), "-shared", "-Wl,-Bsymbolic", "-Wl,--defsym,missing_weak=0", sizes, provider, consumer, "-o", shared)
	for _, tc := range []struct {
		name   string
		inputs []string
	}{
		{"objects", []string{sizes, provider, consumer}},
		{"objects_reversed", []string{provider, sizes, consumer}},
		{"weak_override", []string{weak, sizes, provider, consumer}},
		{"archive", []string{consumer, archive}},
		{"archive_root", []string{rootArchive}},
		{"system_library", []string{shared}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(Options{})
			defer s.Close()
			load(t, s, tc.inputs...)
			if err := s.Link("eval"); err != nil {
				t.Fatal(err)
			}
			call(t, s, "eval", 20, 22, 42)
			if strings.HasPrefix(tc.name, "archive") && len(s.image.objects) != 3 {
				t.Fatalf("unused member extracted: %d objects", len(s.image.objects))
			}
		})
	}
	t.Run("missing_definition_retry", func(t *testing.T) {
		s := New(Options{})
		defer s.Close()
		load(t, s, consumer, sizes)
		if err := s.Link("eval"); err == nil || !strings.Contains(err.Error(), "size unavailable") || s.image != nil {
			t.Fatalf("missing size published: %v", err)
		}
		load(t, s, provider)
		if err := s.Link("eval"); err != nil {
			t.Fatal(err)
		}
		call(t, s, "eval", 20, 22, 42)
	})
}
