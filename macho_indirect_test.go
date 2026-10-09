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

func machoAliasFixture(t *testing.T, target string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aliases.o")
	command(t, compiler(), "--target="+target, "-c", "testdata/macho_aliases.s", "-o", path)
	b, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// LLVM's assembler flattens same-file aliases. Retain a genuine nlist
	// chain so the object linker, rather than the assembler, must follow it.
	chain, _ := machoAliasNlist(t, b, "alias_chain")
	_, targetString := machoAliasNlist(t, b, "alias_eval")
	le.PutUint64(b[chain+8:], uint64(targetString))
	return b
}

func machoAliasNlist(t *testing.T, b []byte, name string) (int, uint32) {
	t.Helper()
	f, err := macho.NewFile(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for i, s := range f.Symtab.Syms {
		if s.Name == "_"+name {
			offset := int(f.ByteOrder.Uint32(f.Symtab.Raw()[8:])) + i*16
			return offset, le.Uint32(b[offset:])
		}
	}
	t.Fatalf("missing Mach-O nlist %s", name)
	return 0, 0
}

func parseMachoAliases(t *testing.T, b []byte) *object {
	t.Helper()
	f, err := parse("aliases.o", b)
	if err != nil || len(f.info.Unsupported) != 0 {
		t.Fatalf("Mach-O aliases: %v, %+v", err, f)
	}
	return f.obj
}

func TestMachOIndirectSymbols(t *testing.T) {
	for _, target := range []string{"x86_64-apple-macos11", "arm64-apple-macos11"} {
		t.Run(target, func(t *testing.T) {
			b := machoAliasFixture(t, target)
			alias, _ := machoAliasNlist(t, b, "alias_eval")
			le.PutUint16(b[alias+6:], 0x80) // N_WEAK_DEF does not weaken N_INDR.
			o := parseMachoAliases(t, b)
			provider := &object{symbols: []symbol{{name: "eval", global: true, weak: true, section: -1, value: 42}}}
			defs, err := definitions([]*object{o, provider})
			if err != nil {
				t.Fatal(err)
			}
			im := &image{defs: defs, external: func(string) uintptr { return 99 }}
			for _, name := range []string{"alias_eval", "alias_chain", "hidden_eval"} {
				d := defs[name]
				if d.sym().weak || d.sym().forward == nil {
					t.Fatalf("%s is not a strong forwarding definition", name)
				}
				if got, err := im.lookup(name); err != nil || got != 42 {
					t.Fatalf("%s address: %d, %v", name, got, err)
				}
			}
			duplicate := &object{symbols: []symbol{{name: "alias_eval", global: true, section: -1}}}
			if _, err := definitions([]*object{o, duplicate}); err == nil || !strings.Contains(err.Error(), "duplicate strong") {
				t.Fatalf("strong alias conflict: %v", err)
			}
			// A local symbol with the target spelling does not satisfy N_INDR.
			index, _ := machoAliasNlist(t, b, "eval")
			b[index+4] &^= 1
			o = parseMachoAliases(t, b)
			defs, err = definitions([]*object{o})
			if err != nil {
				t.Fatal(err)
			}
			im = &image{defs: defs, resolved: map[string]uintptr{}, external: func(name string) uintptr {
				if name == "eval" {
					return 84
				}
				return 0
			}}
			if got, err := im.lookup("alias_chain"); err != nil || got != 84 {
				t.Fatalf("external alias target: %d, %v", got, err)
			}
		})
	}
}

func TestMachOIndirectMalformedAndCycles(t *testing.T) {
	b := machoAliasFixture(t, "arm64-apple-macos11")
	alias, _ := machoAliasNlist(t, b, "alias_eval")
	for _, tc := range []struct {
		name  string
		patch func([]byte)
	}{
		{"offset", func(b []byte) { le.PutUint64(b[alias+8:], ^uint64(0)) }},
		{"empty", func(b []byte) { le.PutUint64(b[alias+8:], 0) }},
		{"section", func(b []byte) { b[alias+5] = 1 }},
		{"terminator", func(b []byte) {
			f, err := macho.NewFile(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			stroff, strsize := uint64(le.Uint32(f.Symtab.Raw()[16:])), uint64(le.Uint32(f.Symtab.Raw()[20:]))
			start := stroff + le.Uint64(b[alias+8:])
			for i := start; i < stroff+strsize; i++ {
				b[i] = 'x'
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := append([]byte(nil), b...)
			tc.patch(bad)
			if _, err := parse("bad-alias", bad); err == nil {
				t.Fatal("accepted malformed N_INDR target")
			}
		})
	}
	// Non-external N_INDR records are ignored, including invalid target offsets.
	local := append([]byte(nil), b...)
	local[alias+4] &^= 1
	le.PutUint64(local[alias+8:], ^uint64(0))
	parseMachoAliases(t, local)
	_, chainString := machoAliasNlist(t, b, "alias_chain")
	le.PutUint64(b[alias+8:], uint64(chainString))
	o := parseMachoAliases(t, b)
	s := New(Options{})
	s.files = []*file{{obj: o}}
	if _, err := s.selectObjects([]string{"alias_chain"}); err == nil || !strings.Contains(err.Error(), "indirect alias cycle") {
		t.Fatalf("cycle validation: %v", err)
	}
}

func TestMachOIndirectDottedTarget(t *testing.T) {
	dir := t.TempDir()
	src, out := filepath.Join(dir, "dotted.s"), filepath.Join(dir, "dotted.o")
	if err := os.WriteFile(src, []byte(".globl _alias\n_alias = __eval.dot\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command(t, compiler(), "--target=arm64-apple-macos11", "-c", src, "-o", out)
	b, err := readFile(out)
	if err != nil {
		t.Fatal(err)
	}
	o := parseMachoAliases(t, b)
	provider := &object{symbols: []symbol{{name: "_eval.dot", global: true, section: -1, value: 42}}}
	defs, err := definitions([]*object{o, provider})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := (&image{defs: defs}).lookup("alias"); err != nil || got != 42 {
		t.Fatalf("dotted alias target: %d, %v", got, err)
	}
}

func TestMachOIndirectArchiveAndLifecycleDependencies(t *testing.T) {
	o := parseMachoAliases(t, machoAliasFixture(t, "arm64-apple-macos11"))
	provider := &object{info: o.info, symbols: []symbol{{name: "eval", global: true, section: -2, size: 4, align: 4}}}
	consumer := &object{symbols: []symbol{{name: "alias_chain", global: true}}, relocs: []relocation{{symbol: 0, pair: -1}}}
	unused := &object{symbols: []symbol{{name: "unused", global: true, section: -1}, {name: "missing", global: true}}, relocs: []relocation{{symbol: 1, pair: -1}}}
	// Exercise selection only on every CI host. Native Mach-O relocation and
	// execution are covered separately on both supported macOS architectures.
	for _, obj := range []*object{o, provider} {
		obj.info.Format = map[string]string{"darwin": "Mach-O", "linux": "ELF", "windows": "COFF"}[runtime.GOOS]
		obj.info.OS, obj.info.Arch, obj.info.Bits = runtime.GOOS, runtime.GOARCH, 64
		if runtime.GOARCH == "386" {
			obj.info.Bits = 32
		}
	}
	s := New(Options{})
	s.files = []*file{{obj: consumer}, {members: []*file{{obj: unused}, {obj: o}, {obj: provider}}}}
	selected, err := s.selectObjects([]string{"alias_chain"})
	if err != nil || len(selected) != 3 {
		t.Fatalf("alias archive extraction: %d, %v", len(selected), err)
	}
	defs, err := definitions(selected)
	if err != nil {
		t.Fatal(err)
	}
	im := &image{objects: selected, defs: defs, common: map[string]uint64{"eval": 64}, base: 0x1000}
	if got, err := im.lookup("alias_chain"); err != nil || got != 0x1040 {
		t.Fatalf("common alias: %#x, %v", got, err)
	}
	order, err := im.lifecycleObjectOrder()
	if err != nil {
		t.Fatal(err)
	}
	if order[selected[2]] >= order[selected[1]] || order[selected[1]] >= order[selected[0]] {
		t.Fatalf("alias initialization dependencies: %v", order)
	}
}

func TestNativeMachOIndirectCalls(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O execution requires macOS")
	}
	needNative(t)
	dir := t.TempDir()
	arch := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH]
	alias := filepath.Join(dir, "aliases.o")
	if err := os.WriteFile(alias, machoAliasFixture(t, arch+"-apple-macos11"), 0600); err != nil {
		t.Fatal(err)
	}
	provider := compile(t, "testdata/macho_alias_provider.c", filepath.Join(dir, "provider.o"))
	consumer := compile(t, "testdata/macho_alias_consumer.c", filepath.Join(dir, "consumer.o"))
	unused := compile(t, "testdata/missing.c", filepath.Join(dir, "unused.o"))
	archive := filepath.Join(dir, "aliases.a")
	command(t, "ar", "rcs", archive, unused, alias, provider)
	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{"objects", []string{consumer, alias, provider}},
		{"consumer_archive", []string{consumer, archive}},
		{"archive_root", []string{archive}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(Options{})
			defer s.Close()
			load(t, s, tc.paths...)
			call(t, s, "alias_chain", 20, 22, 42)
			want, err := s.Lookup("eval")
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"alias_eval", "hidden_eval"} {
				call(t, s, name, 20, 22, 42)
				if got, err := s.Lookup(name); err != nil || got != want {
					t.Fatalf("alias address: %#x, %v", got, err)
				}
			}
			if tc.name != "archive_root" {
				call(t, s, "consume_alias", 20, 22, 126)
			}
		})
	}
	t.Run("host_target", func(t *testing.T) {
		providerSession := New(Options{})
		defer providerSession.Close()
		load(t, providerSession, provider)
		address, err := providerSession.Lookup("eval")
		if err != nil {
			t.Fatal(err)
		}
		s := New(Options{})
		defer s.Close()
		load(t, s, alias)
		if err := s.Define("eval", address); err != nil {
			t.Fatal(err)
		}
		call(t, s, "alias_chain", 20, 22, 42)
		conflict := New(Options{})
		defer conflict.Close()
		load(t, conflict, alias, provider)
		if err := conflict.Define("alias_eval", address); err != nil {
			t.Fatal(err)
		}
		if err := conflict.Link(); err == nil || !strings.Contains(err.Error(), "both host and object") {
			t.Fatalf("host alias definition conflict: %v", err)
		}
	})
}

func TestNativeMachOIndirectRetry(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O execution requires macOS")
	}
	needNative(t)
	dir := t.TempDir()
	alias := filepath.Join(dir, "aliases.o")
	arch := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH]
	if err := os.WriteFile(alias, machoAliasFixture(t, arch+"-apple-macos11"), 0600); err != nil {
		t.Fatal(err)
	}
	s := New(Options{})
	defer s.Close()
	load(t, s, alias)
	if err := s.Link("alias_chain"); err == nil || !strings.Contains(err.Error(), "unresolved symbol eval") {
		t.Fatalf("missing alias target: %v", err)
	}
	if s.image != nil {
		t.Fatal("failed alias target published an image")
	}
	provider := compile(t, "testdata/macho_alias_provider.c", filepath.Join(dir, "provider.o"))
	load(t, s, provider)
	call(t, s, "alias_chain", 20, 22, 42)
}
