package dylib

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func coffAssembly(t *testing.T, target, text string) ([]byte, *object) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "symbols.s")
	if err := os.WriteFile(src, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	out := src + ".o"
	command(t, compiler(), "--target="+target, "-c", src, "-o", out)
	data, err := readFile(out)
	if err != nil {
		t.Fatal(err)
	}
	f, err := parse(out, data)
	if err != nil {
		t.Fatal(err)
	}
	return data, f.obj
}

func weakAuxOffset(t *testing.T, data []byte, name string) int {
	t.Helper()
	f, err := parse("weak", data)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range f.obj.symbols {
		if v.name == name && v.alias != nil {
			return int(le.Uint32(data[8:])) + (i+1)*18
		}
	}
	t.Fatalf("missing weak external %s", name)
	return 0
}

func TestForeignCOFFWeakSearchAndRelocations(t *testing.T) {
	for _, target := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			word := ".quad "
			if strings.HasPrefix(target, "i686") {
				word = ".long "
			}
			data, consumer := coffAssembly(t, target, ".data\n.globl target\ntarget:\n.long 7\n.weak optional\n.set optional,target\n.globl ref\nref:\n"+word+"optional\n.section .rdata,\"dr\"\n.secidx optional\n.secrel32 optional\n")
			_, provider := coffAssembly(t, target, ".data\n.globl optional\noptional:\n.long 42\n")
			// Only the dependency algorithm runs here. Its archive load gate
			// requires host metadata; actual COFF execution is tested on Windows.
			provider.info.Format = map[string]string{"darwin": "Mach-O", "linux": "ELF", "windows": "COFF"}[runtime.GOOS]
			provider.info.OS, provider.info.Arch = runtime.GOOS, runtime.GOARCH
			provider.info.Bits = 64
			if runtime.GOARCH == "386" {
				provider.info.Bits = 32
			}
			for _, mode := range []uint32{1, 2, 3} {
				bytes := append([]byte(nil), data...)
				le.PutUint32(bytes[weakAuxOffset(t, data, "optional")+4:], mode)
				f, err := parse("consumer", bytes)
				if err != nil || len(f.info.Unsupported) != 0 {
					t.Fatalf("weak metadata: %v %+v", err, f)
				}
				s := New(Options{})
				s.files = []*file{f, {members: []*file{{obj: provider}}}}
				selected, err := s.selectObjects([]string{"ref"})
				count := 1
				if mode == 2 {
					count = 2
				}
				if err != nil || len(selected) != count {
					t.Fatalf("search mode %d: %d objects, %v", mode, len(selected), err)
				}
				for _, o := range selected {
					o.info.Format, o.info.OS, o.info.Arch, o.info.Bits = consumer.info.Format, consumer.info.OS, consumer.info.Arch, consumer.info.Bits
				}
				defs, err := definitions(selected)
				if err != nil {
					t.Fatal(err)
				}
				aliases, err := weakAliases(selected)
				if err != nil {
					t.Fatal(err)
				}
				im := &image{base: 0x10000000, objects: selected, defs: defs, aliases: aliases, mem: make([]byte, 4096), external: func(string) uintptr { return 0 }}
				offset := uint64(0)
				for _, o := range selected {
					for _, sec := range o.sections {
						if sec != nil {
							sec.offset = offset
							copy(im.mem[offset:], sec.data)
							offset += 64
						}
					}
				}
				key := "target"
				if mode == 2 {
					key = "optional"
				}
				want, err := im.lookup(key)
				if err != nil {
					t.Fatal(err)
				}
				if got, err := im.lookup("optional"); err != nil || got != want {
					t.Fatalf("alias lookup: %#x, %v", got, err)
				}
				for _, r := range selected[0].relocs {
					if err := im.relocate(selected[0], r); err != nil {
						t.Fatal(err)
					}
					if r.section == defs["ref"].sym().section {
						b := im.mem[selected[0].sections[r.section].offset+r.offset:]
						got := uint64(le.Uint32(b))
						if word == ".quad " {
							got = le.Uint64(b)
						}
						if got != uint64(want) {
							t.Fatalf("weak relocation: %#x, want %#x", got, want)
						}
					}
				}
			}
			// A fallback provider can itself be extracted lazily from an archive.
			for i, v := range consumer.symbols {
				if v.name == "target" {
					consumer.symbols[i].section = 0
				}
			}
			_, fallback := coffAssembly(t, target, ".data\n.globl target\ntarget:\n.long 42\n")
			fallback.info = provider.info
			s := New(Options{})
			s.files = []*file{{obj: consumer}, {members: []*file{{obj: fallback}}}}
			selected, err := s.selectObjects([]string{"ref"})
			if err != nil || len(selected) != 2 {
				t.Fatalf("fallback extraction: %d, %v", len(selected), err)
			}
		})
	}
}

func TestCOFFWeakMalformedAndCycles(t *testing.T) {
	data, _ := coffAssembly(t, "x86_64-pc-windows-msvc", ".data\n.weak optional\n.set optional,target\n.globl target\n.globl ref\nref:\n.quad optional\n")
	offset := weakAuxOffset(t, data, "optional")
	for _, patch := range []func([]byte){
		func(b []byte) { le.PutUint32(b[offset:], 0xffffffff) },
		func(b []byte) { le.PutUint32(b[offset:], uint32((offset-int(le.Uint32(b[8:])))/18)) },
		func(b []byte) { le.PutUint32(b[offset+4:], 4) },
		func(b []byte) { b[offset-1] = 0 },
	} {
		b := append([]byte(nil), data...)
		patch(b)
		if _, err := parse("bad-weak", b); err == nil {
			t.Fatal("accepted malformed weak external")
		}
	}
	_, o := coffAssembly(t, "x86_64-pc-windows-msvc", ".data\n.weak optional\n.set optional,middle\n.weak middle\n.set middle,target\n.globl target\n.globl ref\nref:\n.quad optional\n")
	var optional, middle int
	for i, v := range o.symbols {
		if v.name == "optional" {
			optional = i
		}
		if v.name == "middle" {
			middle = i
		}
	}
	o.symbols[optional].alias.target = middle
	o.symbols[middle].alias.target = optional
	s := New(Options{})
	s.files = []*file{{obj: o}}
	if _, err := s.selectObjects([]string{"optional"}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("weak cycle: %v", err)
	}
}

func TestCOFFWeakChainsAndLifecycle(t *testing.T) {
	consumer := &object{info: Info{Format: "COFF"}, symbols: []symbol{
		{name: "optional", global: true, weak: true, alias: &weakAlias{target: 1, search: 3}},
		{name: "middle", global: true, weak: true, alias: &weakAlias{target: 2, search: 3}},
		{name: "target", global: true},
	}, relocs: []relocation{{symbol: 0, pair: -1}}}
	provider := &object{symbols: []symbol{{name: "target", global: true, section: 1}}}
	strong := &object{symbols: []symbol{{name: "optional", global: true, section: 1}}}
	for _, tc := range []struct {
		name     string
		objects  []*object
		external uintptr
		want     *object
	}{
		{"fallback chain", []*object{consumer, provider}, 0, provider},
		{"strong override", []*object{consumer, provider, strong}, 0, strong},
		{"external override", []*object{consumer, provider}, 123, consumer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defs, err := definitions(tc.objects)
			if err != nil {
				t.Fatal(err)
			}
			aliases, err := weakAliases(tc.objects)
			if err != nil {
				t.Fatal(err)
			}
			im := &image{objects: tc.objects, defs: defs, aliases: aliases, external: func(name string) uintptr {
				if name == "optional" {
					return tc.external
				}
				return 0
			}}
			d, err := im.resolveDefinition(consumer, 0)
			if err != nil || d.o != tc.want {
				t.Fatalf("resolved provider: %+v, %v", d, err)
			}
			order, err := im.lifecycleObjectOrder()
			if err != nil || order[tc.want] != 0 {
				t.Fatalf("initialization order: %v, %v", order, err)
			}
		})
	}
	// A relocation in another object may use an ordinary undefined symbol.
	// Alias resolution is by public name, not only by the local weak record.
	ref := &object{symbols: []symbol{{name: "optional", global: true}}}
	aliases, err := weakAliases([]*object{consumer})
	if err != nil {
		t.Fatal(err)
	}
	d, err := (&image{defs: map[string]definition{"target": {provider, 0}}, aliases: aliases}).resolveDefinition(ref, 0)
	if err != nil || d.o != provider {
		t.Fatalf("ordinary reference to alias: %+v, %v", d, err)
	}
}

func TestNativeCOFFWeakCalls(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("COFF execution needs Windows")
	}
	needNative(t)
	dir := t.TempDir()
	weak := compile(t, "testdata/coff_weak.c", filepath.Join(dir, "weak.obj"))
	providerSource := filepath.Join(dir, "provider.c")
	if err := os.WriteFile(providerSource, []byte("int optional(int a,int b){return a+b+1;}"), 0600); err != nil {
		t.Fatal(err)
	}
	provider := compile(t, providerSource, filepath.Join(dir, "provider.obj"))
	archive := filepath.Join(dir, "provider.lib")
	command(t, "ar", "rcs", archive, provider)
	for _, tc := range []struct {
		name  string
		mode  uint32
		extra string
		want  int32
	}{
		{"fallback", 3, "", 42}, {"explicit", 3, provider, 43}, {"alias archive", 3, archive, 42}, {"no-library", 1, archive, 42}, {"library", 2, archive, 43},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := readFile(weak)
			if err != nil {
				t.Fatal(err)
			}
			le.PutUint32(data[weakAuxOffset(t, data, "optional")+4:], tc.mode)
			path := filepath.Join(dir, tc.name+".obj")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			s := New(Options{})
			defer s.Close()
			load(t, s, path)
			if tc.extra != "" {
				load(t, s, tc.extra)
			}
			call(t, s, "call_weak", 20, 22, tc.want)
		})
	}
}
