package dylib

import (
	"bytes"
	"debug/macho"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// LLVM's assembler intentionally has no spelling for this linker-synthesized
// section type. Compact a genuine Clang pointer table into ld64's uint32
// original-image offsets, retaining code/data relocations and all other tables.
func machoOffsetEncoding(t *testing.T, b []byte) []byte {
	t.Helper()
	f := machoPointerMetadata(t, b)
	s := f.Section("__mod_init_func")
	if s == nil || s.Flags&0xff != 9 || s.Size == 0 || s.Size%8 != 0 {
		t.Fatal("missing initializer pointer fixture")
	}
	data, err := s.Data()
	if err != nil {
		t.Fatal(err)
	}
	values := make([]uint64, s.Size/8)
	seen := make(map[uint32]bool)
	for _, r := range s.Relocs {
		if r.Type != 0 || r.Len != 3 || r.Pcrel || r.Scattered || r.Addr%8 != 0 || uint64(r.Addr) >= s.Size || seen[r.Addr] {
			t.Fatal("unexpected initializer pointer relocation")
		}
		seen[r.Addr] = true
		value := le.Uint64(data[r.Addr:])
		if r.Extern {
			value += f.Symtab.Syms[r.Value].Value
		}
		if value > math.MaxUint32 {
			t.Fatal("fixture initializer is not a uint32 original image offset")
		}
		values[r.Addr/8] = value
	}
	if len(seen) != len(values) {
		t.Fatal("incomplete initializer pointer relocations")
	}
	h := machoPointerHeader(t, b, s.Name)
	out := bytes.Clone(b)
	for i, value := range values {
		le.PutUint32(out[int(s.Offset)+i*4:], uint32(value))
	}
	clear(out[h+16 : h+32])
	copy(out[h+16:h+32], "__TEXT")
	le.PutUint64(out[h+40:], uint64(len(values)*4))
	le.PutUint32(out[h+52:], 2)
	le.PutUint32(out[h+56:], 0) // Offset tables have no relocations.
	le.PutUint32(out[h+60:], 0)
	le.PutUint32(out[h+64:], s.Flags&^0xff|0x16)
	return out
}

func machoOffsetFixture(t *testing.T, target string, optimization string) []byte {
	t.Helper()
	path := compile(t, "testdata/lifecycle_macho_offsets.c", filepath.Join(t.TempDir(), "offsets.o"), "--target="+target, optimization)
	b, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return machoOffsetEncoding(t, b)
}

func machoOffsetPreferredBase(t *testing.T, b []byte, base uint64) []byte {
	t.Helper()
	f := machoPointerMetadata(t, b)
	out := bytes.Clone(b)
	position := 32
	for _, load := range f.Loads {
		raw := load.Raw()
		if macho.LoadCmd(le.Uint32(raw)) == macho.LoadCmdSegment64 {
			clear(out[position+8 : position+24])
			copy(out[position+8:position+24], "__TEXT")
			le.PutUint64(out[position+24:], le.Uint64(raw[24:])+base)
			for i := 0; i < int(le.Uint32(raw[64:])); i++ {
				address := position + 72 + i*80 + 32
				le.PutUint64(out[address:], le.Uint64(out[address:])+base)
			}
		}
		position += len(raw)
	}
	symbolStart := int(le.Uint32(f.Symtab.Raw()[8:]))
	for i, s := range f.Symtab.Syms {
		if s.Type&0xe0 == 0 && s.Type&0x0e == 0xe {
			le.PutUint64(out[symbolStart+i*16+8:], s.Value+base)
		}
	}
	return out
}

func TestMachOInitializerOffsetsMetadata(t *testing.T) {
	for _, target := range []string{"x86_64-apple-macosx11", "arm64-apple-macosx11"} {
		t.Run(target, func(t *testing.T) {
			b := machoOffsetFixture(t, target, "-O0")
			// The actual original address must be aligned. A weaker declared
			// alignment is harmless: the loader independently places pages.
			h := machoPointerHeader(t, b, "__mod_init_func")
			le.PutUint32(b[h+52:], 0)
			for _, base := range []uint64{0, 0x10000} {
				f, err := parse("offsets.o", machoOffsetPreferredBase(t, b, base))
				if err != nil || len(f.info.Unsupported) != 0 {
					t.Fatalf("offset metadata: %v %v", f, err)
				}
				var offsets, pointers uint64
				for _, s := range f.obj.sections {
					if s == nil || s.lifecycle != lifecycleInit {
						continue
					}
					if s.initOffsets {
						offsets += s.size / 4
						if s.write || s.exec || le.Uint32(s.data) != 0 {
							t.Fatal("offset-zero initializer or read-only storage was lost")
						}
					} else {
						pointers += s.size / 8
					}
				}
				if offsets != 2 || pointers != 1 {
					t.Fatalf("tables: %d offsets, %d pointers", offsets, pointers)
				}
				if f.obj.originalBase != base {
					t.Fatal("preferred image base was lost")
				}
			}
		})
	}
}

func TestMachOInitializerOffsetsValidation(t *testing.T) {
	b := machoOffsetFixture(t, "arm64-apple-macosx11", "-O0")
	h := machoPointerHeader(t, b, "__mod_init_func")
	for _, tc := range []struct {
		name string
		edit func([]byte)
	}{
		{"partial-entry", func(b []byte) { le.PutUint64(b[h+40:], 7) }},
		{"misaligned-address", func(b []byte) { le.PutUint64(b[h+32:], le.Uint64(b[h+32:])+1) }},
		{"writable", func(b []byte) { clear(b[h+16 : h+32]); copy(b[h+16:h+32], "__DATA") }},
		{"executable", func(b []byte) { le.PutUint32(b[h+64:], le.Uint32(b[h+64:])|0x80000000) }},
		{"unresolved-relocation", func(b []byte) {
			original := machoPointerMetadata(t, b).Section("__other_inits")
			le.PutUint32(b[h+56:], original.Reloff)
			le.PutUint32(b[h+60:], original.Nreloc)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := bytes.Clone(b)
			tc.edit(data)
			if _, err := parse("bad-offsets.o", data); err == nil {
				t.Fatal("malformed offset table accepted")
			}
		})
	}
}

func TestMachOInitializerOffsetsRebasing(t *testing.T) {
	for _, originalBase := range []uint64{0, 0x10000} {
		for _, arch := range []string{"amd64", "arm64"} {
			mem := make([]byte, 256)
			first := &section{name: "__text", size: 16, align: 4, exec: true, original: originalBase, offset: 128}
			second := &section{name: "__more_code", size: 16, align: 4, exec: true, original: originalBase + 32, offset: 192}
			table := &section{name: "__init_offsets", size: 8, align: 4, original: originalBase + 64, lifecycle: lifecycleInit, initOffsets: true}
			pointer := &section{name: "__normal_init", size: 8, align: 8, original: originalBase + 72, offset: 8, lifecycle: lifecycleInit}
			le.PutUint32(mem, 0) // A function, not a null pointer sentinel.
			le.PutUint32(mem[4:], 36)
			le.PutUint64(mem[8:], 0x1084)
			o := &object{info: Info{Format: "Mach-O", Arch: arch, Bits: 64}, originalBase: originalBase, sections: []*section{nil, first, table, second, pointer}}
			im := &image{mem: mem, base: 0x1000, pointerSize: 8, objects: []*object{o}}
			if err := im.rebaseMachOInitializerOffsets(); err != nil {
				t.Fatal(err)
			}
			if le.Uint32(mem) != 128 || le.Uint32(mem[4:]) != 196 {
				t.Fatal("independently placed code sections were not rebased")
			}
			if err := im.prepareLifecycle(); err != nil || !reflect.DeepEqual(im.initializers, []uintptr{0x1084, 0x1080, 0x10c4}) {
				t.Fatalf("pointer/offset order or zero offset: %v %v", im.initializers, err)
			}
			le.PutUint32(mem, 0xffffffff)
			if err := im.prepareLifecycle(); err == nil {
				t.Fatal("out-of-image normalized offset accepted")
			}
			if arch == "arm64" {
				le.PutUint32(mem, 129)
				if err := im.prepareLifecycle(); err == nil {
					t.Fatal("unaligned ARM64 initializer accepted")
				}
			}
		}
	}
	for _, bad := range []struct {
		name  string
		base  uint64
		extra *section
		value uint32
	}{
		{name: "data", value: 32},
		{name: "one-past-code", value: 16},
		{name: "outside", value: math.MaxUint32},
		{name: "ambiguous", extra: &section{original: 0, size: 16, exec: true}},
		{name: "overflow", base: math.MaxUint64, value: 1},
	} {
		t.Run(bad.name, func(t *testing.T) {
			mem := make([]byte, 64)
			le.PutUint32(mem, bad.value)
			table := &section{name: "offsets", size: 4, align: 4, initOffsets: true, lifecycle: lifecycleInit, original: 32}
			o := &object{originalBase: bad.base, sections: []*section{nil, {size: 16, exec: true}, table, bad.extra}}
			if err := (&image{mem: mem, objects: []*object{o}}).rebaseMachOInitializerOffsets(); err == nil {
				t.Fatal("invalid original initializer offset accepted")
			}
		})
	}
}

func TestNativeMachOInitializerOffsets(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O execution requires macOS")
	}
	needNative(t)
	target := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH] + "-apple-macosx11"
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			b := machoOffsetFixture(t, target, optimization)
			dir := t.TempDir()
			obj := filepath.Join(dir, "offsets.o")
			if err := os.WriteFile(obj, b, 0600); err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(dir, "offsets.a")
			command(t, "ar", "rcs", archive, obj)
			for _, input := range []string{obj, archive} {
				t.Run(filepath.Ext(input), func(t *testing.T) {
					host, _, read := lifecycleObserver(t, t.TempDir())
					s := New(Options{})
					defer s.Close()
					load(t, s, input)
					if err := s.Link("offset_root"); err == nil {
						t.Fatal("missing initializer dependency accepted")
					}
					expectEvents(t, read)
					load(t, s, host)
					if err := s.Link("absent_root"); err == nil {
						t.Fatal("missing root accepted")
					}
					expectEvents(t, read)
					call(t, s, "offset_root", 0, 0, 42)
					call(t, s, "offset_root", 0, 0, 42)
					expectEvents(t, read, 2, 3, 4)
					if err := s.Close(); err != nil {
						t.Fatal(err)
					}
					expectEvents(t, read, 2, 3, 4, 5, 6)
				})
			}
		})
	}
}

func TestNativeMachOInitializerOffsetsBeforeExecution(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O execution requires macOS")
	}
	needNative(t)
	target := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH] + "-apple-macosx11"
	b := machoOffsetFixture(t, target, "-O0")
	h := machoPointerHeader(t, b, "__mod_init_func")
	entry := int(le.Uint32(b[h+48:])) + 4
	badValues := []uint32{math.MaxUint32, uint32(machoPointerMetadata(t, b).Section("__mod_term_func").Addr)}
	if runtime.GOARCH == "arm64" {
		badValues = append(badValues, 1)
	}
	for _, value := range badValues {
		data := bytes.Clone(b)
		le.PutUint32(data[entry:], value)
		path := filepath.Join(t.TempDir(), "bad.o")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		host, _, read := lifecycleObserver(t, t.TempDir())
		s := New(Options{})
		load(t, s, host, path)
		if err := s.Link("offset_root"); err == nil || !strings.Contains(err.Error(), "outside executable") || s.image != nil {
			t.Fatalf("invalid offset accepted or published: %v", err)
		}
		expectEvents(t, read) // Even the valid pointer initializer must not run.
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		expectEvents(t, read)
	}
}

func TestNativeMachOInitializerOffsetsDependencies(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O execution requires macOS")
	}
	needNative(t)
	dir := t.TempDir()
	flags := []string{"-std=c++17", "-fno-exceptions", "-fno-rtti"}
	consumer := compile(t, "testdata/lifecycle_consumer.cpp", filepath.Join(dir, "consumer.o"), flags...)
	provider := compile(t, "testdata/lifecycle_provider.cpp", filepath.Join(dir, "provider.o"), flags...)
	unused := compile(t, "testdata/lifecycle_unused.cpp", filepath.Join(dir, "unused.o"), flags...)
	convert := func(path string) string {
		data, err := readFile(path)
		if err != nil {
			t.Fatal(err)
		}
		out := path + ".offset.o"
		if err := os.WriteFile(out, machoOffsetEncoding(t, data), 0600); err != nil {
			t.Fatal(err)
		}
		return out
	}
	consumerOffsets, providerOffsets, unusedOffsets := convert(consumer), convert(provider), convert(unused)
	// A semantically invalid table in an unselected member must not execute
	// or block loading the archive. Selected entries are checked during Link.
	data, err := readFile(unusedOffsets)
	if err != nil {
		t.Fatal(err)
	}
	h := machoPointerHeader(t, data, "__mod_init_func")
	le.PutUint32(data[le.Uint32(data[h+48:]):], math.MaxUint32)
	if err := os.WriteFile(unusedOffsets, data, 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "providers.a")
	command(t, "ar", "rcs", archive, unusedOffsets, providerOffsets)
	for i, inputs := range [][]string{{consumer, providerOffsets}, {consumerOffsets, provider}, {consumerOffsets, archive}} {
		t.Run([]string{"offset-provider", "offset-consumer", "offset-archive"}[i], func(t *testing.T) {
			host, _, read := lifecycleObserver(t, t.TempDir())
			s := New(Options{})
			defer s.Close()
			load(t, s, host)
			load(t, s, inputs...)
			call(t, s, "initialized_add", 0, 0, 42)
			expectEvents(t, read, 1, 2)
			call(t, s, "register_later", 20, 22, 42)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			expectEvents(t, read, 1, 2, 7, 8, 9)
		})
	}
}
