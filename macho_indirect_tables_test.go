package dylib

import (
	"bytes"
	"debug/macho"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func machoPointerFixture(t *testing.T, target string, flags ...string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pointers.o")
	args := []string{"--target=" + target, "-c", "testdata/macho_pointers.S", "-o", path}
	command(t, compiler(), append(args, flags...)...)
	b, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func machoPointerMetadata(t *testing.T, b []byte) *macho.File {
	t.Helper()
	f, err := macho.NewFile(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func machoPointerHeader(t *testing.T, b []byte, name string) int {
	t.Helper()
	f := machoPointerMetadata(t, b)
	offset, ordinal := 32, 0
	for _, load := range f.Loads {
		raw := load.Raw()
		if macho.LoadCmd(le.Uint32(raw)) == macho.LoadCmdSegment64 {
			for i := 0; i < int(le.Uint32(raw[64:])); i++ {
				if f.Sections[ordinal].Name == name {
					return offset + 72 + i*80
				}
				ordinal++
			}
		}
		offset += len(raw)
	}
	t.Fatalf("missing section header %s", name)
	return 0
}

func machoPointerEntry(t *testing.T, b []byte, section string, entry int) int {
	t.Helper()
	f := machoPointerMetadata(t, b)
	h := machoPointerHeader(t, b, section)
	return int(f.Dysymtab.Indirectsymoff) + (int(le.Uint32(b[h+68:]))+entry)*4
}

func TestMachOIndirectTables(t *testing.T) {
	for _, target := range []string{"x86_64-apple-macos11", "arm64-apple-macos11"} {
		t.Run(target, func(t *testing.T) {
			b := machoPointerFixture(t, target)
			o := parseMachoAliases(t, b)
			provider := &object{symbols: []symbol{{name: "eval", global: true, section: -1, value: 0x12345678}}}
			defs, err := definitions([]*object{o, provider})
			if err != nil {
				t.Fatal(err)
			}
			im := &image{objects: []*object{o}, defs: defs, base: 0x100000, mem: make([]byte, 8192), got: map[uintptr]uintptr{}, gotStart: 4096, gotNext: 4096, stubStart: 8192, pointerSize: 8}
			for i, sec := range o.sections {
				if sec != nil {
					sec.offset = uint64(i) * 128
					copy(im.mem[sec.offset:], sec.data)
				}
			}
			for _, r := range o.relocs {
				if err := im.relocate(o, r); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"lazy_eval", "nonlazy_eval"} {
				d := defs[name]
				sec := o.sections[d.sym().section]
				if got := le.Uint64(im.mem[sec.offset+d.sym().value:]); got != 0x12345678 {
					t.Fatalf("%s pointer: %#x", name, got)
				}
			}
			local := defs["local_pointer"]
			got := le.Uint64(im.mem[local.o.sections[local.sym().section].offset+local.sym().value:])
			if got < uint64(im.base) || le.Uint32(im.mem[got-uint64(im.base):]) != 42 {
				t.Fatalf("local pointer rebasing: %#x", got)
			}
			stub := defs["stub_eval"]
			code := im.mem[o.sections[stub.sym().section].offset+stub.sym().value:]
			if o.info.Arch == "amd64" {
				if code[0] != 0xff || code[1] != 0x25 {
					t.Fatal("stub is not an indirect tail jump")
				}
				place, err := im.lookup("stub_eval")
				if err != nil {
					t.Fatal(err)
				}
				got := int64(place) + 6 + int64(int32(le.Uint32(code[2:])))
				if got != int64(im.base)+4096 {
					t.Fatalf("stub GOT address: %#x", got)
				}
			} else if le.Uint32(code[8:]) != 0xd61f0200 {
				t.Fatal("stub does not preserve the link register")
			}
		})
	}
}

func TestMachOIndirectTableMalformed(t *testing.T) {
	b := machoPointerFixture(t, "arm64-apple-macos11")
	nonlazy := machoPointerHeader(t, b, "__nl_symbol_ptr")
	stub := machoPointerHeader(t, b, "__stubs")
	entry := machoPointerEntry(t, b, "__nl_symbol_ptr", 0)
	local := machoPointerEntry(t, b, "__nl_symbol_ptr", 1)
	for _, tc := range []struct {
		name  string
		patch func([]byte)
	}{
		{"table_start", func(b []byte) { le.PutUint32(b[nonlazy+68:], 0xffffffff) }},
		{"partial_entry", func(b []byte) { le.PutUint64(b[nonlazy+40:], 15) }},
		{"zero_stub", func(b []byte) { le.PutUint32(b[stub+72:], 0) }},
		{"symbol_index", func(b []byte) { le.PutUint32(b[entry:], 0x3fffffff) }},
		{"abs_without_local", func(b []byte) { le.PutUint32(b[entry:], machoIndirectAbs) }},
		{"local_stub", func(b []byte) { le.PutUint32(b[machoPointerEntry(t, b, "__stubs", 0):], machoIndirectLocal) }},
		{"abs_relocation", func(b []byte) { le.PutUint32(b[local:], machoIndirectLocal|machoIndirectAbs) }},
		{"missing_table", func(b []byte) {
			f := machoPointerMetadata(t, b)
			offset := bytes.Index(b, f.Dysymtab.Raw())
			le.PutUint32(b[offset+60:], 0)
		}},
		{"stripped_local_range", func(b []byte) {
			le.PutUint32(b[nonlazy+60:], 0)
			f := machoPointerMetadata(t, b)
			le.PutUint64(b[int(f.Section("__nl_symbol_ptr").Offset)+8:], ^uint64(0))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := append([]byte(nil), b...)
			tc.patch(bad)
			if _, err := parse("bad-pointers", bad); err == nil {
				t.Fatal("accepted malformed indirect table")
			}
		})
	}
	bad := append([]byte(nil), b...)
	le.PutUint32(bad[stub+72:], 4)
	f, err := parse("custom-stub", bad)
	if err != nil || len(f.info.Unsupported) == 0 || !strings.Contains(strings.Join(f.info.Unsupported, ","), "symbol stubs of width 4") {
		t.Fatalf("custom stub scope: %v, %+v", err, f)
	}
	// Absolute local entries keep their numeric word; lazy-dylib pointers
	// receive the same eager binding as ordinary lazy pointers.
	b = append([]byte(nil), b...)
	le.PutUint32(b[entry:], machoIndirectLocal|machoIndirectAbs)
	lazy := machoPointerHeader(t, b, "__la_symbol_ptr")
	le.PutUint32(b[lazy+64:], le.Uint32(b[lazy+64:])&^0xff|0x10)
	o := parseMachoAliases(t, b)
	for _, r := range o.relocs {
		if o.sections[r.section].name == "__nl_symbol_ptr" && r.offset == 0 {
			t.Fatal("absolute indirect address was rebound")
		}
	}
}

func TestNativeMachOIndirectPointersAndStubs(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O execution requires macOS")
	}
	needNative(t)
	dir := t.TempDir()
	arch := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH]
	input := filepath.Join(dir, "pointers.o")
	if err := os.WriteFile(input, machoPointerFixture(t, arch+"-apple-macos11"), 0600); err != nil {
		t.Fatal(err)
	}
	provider := compile(t, "testdata/macho_alias_provider.c", filepath.Join(dir, "provider.o"))
	consumer := compile(t, "testdata/macho_pointer_consumer.c", filepath.Join(dir, "consumer.o"))
	unused := compile(t, "testdata/missing.c", filepath.Join(dir, "unused.o"))
	archive := filepath.Join(dir, "pointers.a")
	command(t, "ar", "rcs", archive, unused, input, provider)
	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{"objects", []string{consumer, input, provider}}, {"archive", []string{consumer, archive}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(Options{})
			defer s.Close()
			load(t, s, tc.paths...)
			for i := int32(0); i < 5; i++ {
				call(t, s, "stub_eval", 20+i, 22, 42+i)
				call(t, s, "consume_pointers", 20+i, 22, 168+3*i)
			}
		})
	}
	t.Run("large_records_and_floating_registers", func(t *testing.T) {
		records := compile(t, "testdata/macho_pointer_records.c", filepath.Join(dir, "records.o"))
		s := New(Options{})
		defer s.Close()
		load(t, s, input, records)
		for i := int32(0); i < 5; i++ {
			call(t, s, "consume_records", 20+i, 22, 132+3*i)
		}
	})
	for _, mode := range []string{"stripped_local", "lazy_dylib"} {
		t.Run(mode, func(t *testing.T) {
			b := machoPointerFixture(t, arch+"-apple-macos11")
			if mode == "stripped_local" {
				f := machoPointerMetadata(t, b)
				for _, sym := range f.Symtab.Syms {
					if sym.Name == "_local_number" {
						// The assembler leaves a zero word for its relocation.
						// A stripped local entry contains the linked original address.
						le.PutUint64(b[int(f.Section("__nl_symbol_ptr").Offset)+8:], sym.Value)
					}
				}
				h := machoPointerHeader(t, b, "__nl_symbol_ptr")
				le.PutUint32(b[h+60:], 0) // Remove local fixups, retaining the original object address.
			} else {
				h := machoPointerHeader(t, b, "__la_symbol_ptr")
				le.PutUint32(b[h+64:], le.Uint32(b[h+64:])&^0xff|0x10)
			}
			path := filepath.Join(dir, mode+".o")
			if err := os.WriteFile(path, b, 0600); err != nil {
				t.Fatal(err)
			}
			s := New(Options{})
			defer s.Close()
			load(t, s, consumer, path, provider)
			call(t, s, "consume_pointers", 20, 22, 168)
		})
	}
	t.Run("host_target", func(t *testing.T) {
		host := New(Options{})
		defer host.Close()
		load(t, host, provider)
		address, err := host.Lookup("eval")
		if err != nil {
			t.Fatal(err)
		}
		s := New(Options{})
		defer s.Close()
		load(t, s, consumer, input)
		if err := s.Link("consume_pointers"); err == nil || !strings.Contains(err.Error(), "unresolved symbol eval") {
			t.Fatalf("missing indirect target: %v", err)
		}
		if s.image != nil {
			t.Fatal("missing indirect target published an image")
		}
		if err := s.Define("eval", address); err != nil {
			t.Fatal(err)
		}
		call(t, s, "consume_pointers", 20, 22, 168)
	})
	t.Run("forwarding_alias_target", func(t *testing.T) {
		aliasPath, pointerPath := filepath.Join(dir, "aliases.o"), filepath.Join(dir, "alias-pointers.o")
		if err := os.WriteFile(aliasPath, machoAliasFixture(t, arch+"-apple-macos11"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pointerPath, machoPointerFixture(t, arch+"-apple-macos11", "-DEVAL_TARGET=_alias_eval"), 0600); err != nil {
			t.Fatal(err)
		}
		s := New(Options{})
		defer s.Close()
		load(t, s, consumer, pointerPath, aliasPath, provider)
		call(t, s, "consume_pointers", 20, 22, 168)
	})
}
