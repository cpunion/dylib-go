package dylib

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestELFX86NarrowRelocationFields(t *testing.T) {
	for _, arch := range []string{"amd64", "386"} {
		start := uint32(12)
		if arch == "386" {
			start = 20
		}
		for typ := start; typ < start+4; typ++ {
			o := &object{info: Info{Format: "ELF", Arch: arch}}
			width, relative := elfX86Narrow(o, typ)
			bits := uint(width * 8)
			minimum, maximum := -int64(1)<<(bits-1), int64(1)<<bits-1
			if relative {
				maximum = int64(1)<<(bits-1) - 1
			}
			if arch == "386" && typ == 21 {
				minimum, maximum = -1<<16, 1<<16-1
			}
			for _, implicit := range []bool{false, true} {
				for _, value := range []int64{minimum - 1, minimum, -1, 0, maximum, maximum + 1} {
					mem := bytes.Repeat([]byte{0xa5}, 8)
					clear(mem[2 : 2+width])
					r := relocation{section: 1, offset: 2, typ: typ, pair: -1, implicit: implicit, addend: value + 2}
					if implicit {
						if width == 1 {
							mem[2] = 0xfe
						} else {
							le.PutUint16(mem[2:], 0xfffe)
						}
					} else {
						r.addend = value
					}
					before := bytes.Clone(mem)
					o.sections = []*section{nil, {size: uint64(2 + width)}}
					o.symbols = []symbol{{section: -1, value: 0x1000}}
					base := uintptr(0x1000)
					if relative {
						base -= 2
					} else {
						o.symbols[0].value = 0
					}
					err := (&image{mem: mem, base: base}).relocate(o, r)
					valid := value >= minimum && value <= maximum
					if (err == nil) != valid {
						t.Fatalf("%s type %d implicit=%v value=%d: %v", arch, typ, implicit, value, err)
					}
					if !valid && !bytes.Equal(mem, before) {
						t.Fatal("overflow changed native bytes")
					}
					if !bytes.Equal(mem[:2], before[:2]) || !bytes.Equal(mem[2+width:], before[2+width:]) {
						t.Fatal("narrow relocation changed neighboring bytes")
					}
					if valid {
						got := uint16(mem[2])
						if width == 2 {
							got = le.Uint16(mem[2:])
						}
						mask := uint16(0xffff)
						if width == 1 {
							mask = 0xff
						}
						if got != uint16(value)&mask {
							t.Fatalf("encoded %d, want %d", got, uint16(value)&mask)
						}
					}
					o.sections[1].size--
					if err := (&image{mem: mem, base: base}).relocate(o, r); err == nil {
						t.Fatal("partial relocation field accepted")
					}
				}
			}
		}
	}
}

func TestELFX86NarrowMetadata(t *testing.T) {
	for _, target := range []string{"x86_64-linux-gnu", "i686-linux-gnu"} {
		t.Run(target, func(t *testing.T) {
			path := compile(t, "testdata/elf_x86_narrow.S", filepath.Join(t.TempDir(), "narrow.o"), "--target="+target)
			data, err := readFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(path, data)
			if err != nil {
				t.Fatal(err)
			}
			seen := make(map[uint32]int)
			for _, r := range f.obj.relocs {
				width, _ := elfX86Narrow(f.obj, r.typ)
				if width == 0 || r.implicit != (f.info.Bits == 32) {
					t.Fatalf("unexpected relocation: %+v", r)
				}
				seen[r.typ]++
			}
			if len(seen) != 4 || len(f.obj.relocs) != 6 {
				t.Fatalf("narrow relocations: %v", seen)
			}
		})
	}
}

func TestELF386NarrowAddressWrapping(t *testing.T) {
	for _, typ := range []uint32{21, 23} {
		o := &object{info: Info{Format: "ELF", Arch: "386"}, sections: []*section{nil, {size: 2}}, symbols: []symbol{{section: -1, value: 0}}}
		mem := make([]byte, 2)
		r := relocation{section: 1, typ: typ, pair: -1}
		im := &image{mem: mem, base: 0xfffffff0}
		if err := im.relocate(o, r); err != nil {
			t.Fatal(err)
		}
		if mem[0] != 16 || mem[1] != 0 {
			t.Fatalf("wrapped displacement: %v", mem)
		}
	}
	o := &object{info: Info{Format: "ELF", Arch: "386"}, sections: []*section{nil, {size: 1}}, symbols: []symbol{{section: -1, value: 0xffffffff}}}
	mem := []byte{0xa5}
	if err := (&image{mem: mem}).relocate(o, relocation{section: 1, typ: 22, pair: -1, addend: 1}); err != nil || mem[0] != 0 {
		t.Fatalf("wrapped absolute: %v %v", mem, err)
	}
}

func TestNativeELFX86NarrowRelocations(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" && runtime.GOARCH != "386" {
		t.Skip("x86 ELF execution requires Linux")
	}
	needNative(t)
	dir := t.TempDir()
	data := compile(t, "testdata/elf_x86_narrow.S", filepath.Join(dir, "narrow.o"))
	provider := compile(t, "testdata/elf_x86_narrow_provider.s", filepath.Join(dir, "provider.o"))
	consumer := compile(t, "testdata/elf_x86_narrow_consumer.c", filepath.Join(dir, "consumer.o"))
	archive := filepath.Join(dir, "narrow.a")
	command(t, "ar", "rcs", archive, data, provider)
	for i, inputs := range [][]string{{consumer, data, provider}, {consumer, archive}} {
		t.Run([]string{"objects", "archive"}[i], func(t *testing.T) {
			s := New(Options{})
			defer s.Close()
			load(t, s, inputs...)
			call(t, s, "narrow_root", 20, 22, 42)
		})
	}
	t.Run("dependency-retry", func(t *testing.T) {
		s := New(Options{})
		defer s.Close()
		load(t, s, consumer, data)
		if err := s.Link("narrow_root"); err == nil || s.image != nil {
			t.Fatal("missing absolute provider accepted")
		}
		load(t, s, provider)
		call(t, s, "narrow_root", 20, 22, 42)
	})
	// A high host address cannot be represented by a narrow absolute field.
	for _, bad := range []struct{ byteValue, wordValue uintptr }{{256 + 2, 4244}, {44, 0x10000 + 2}} {
		s := New(Options{})
		load(t, s, consumer, data)
		if err := s.Define("narrow_value8", bad.byteValue); err != nil {
			t.Fatal(err)
		}
		if err := s.Define("narrow_value16", bad.wordValue); err != nil {
			t.Fatal(err)
		}
		if err := s.Link("narrow_root"); err == nil || !strings.Contains(err.Error(), "overflow") || s.image != nil {
			t.Fatalf("narrow overflow accepted: %v", err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
	// Keep the native fixture self-contained for system-linker comparisons.
	main := filepath.Join(dir, "main.c")
	if err := os.WriteFile(main, []byte("extern int narrow_root(int,int); int main(void){return narrow_root(20,22)!=42;}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "reference")
	command(t, compiler(), "-no-pie", main, consumer, data, provider, "-o", out)
	command(t, out)
}
