package dylib

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMissingWeakDoesNotResolveStrong(t *testing.T) {
	for _, tc := range []struct {
		format, arch, name string
	}{
		{"ELF", "amd64", "optional"}, {"ELF", "arm64", "optional"}, {"ELF", "386", "optional"},
		{"Mach-O", "amd64", "optional"}, {"Mach-O", "arm64", "optional"},
		{"COFF", "amd64", "optional"}, {"COFF", "arm64", "optional"}, {"COFF", "386", "optional"},
		{"COFF", "amd64", "__imp_optional"}, {"COFF", "arm64", "__imp_optional"}, {"COFF", "386", "__imp_optional"},
	} {
		t.Run(tc.format+"/"+tc.arch+"/"+tc.name, func(t *testing.T) {
			info := Info{Format: tc.format, Arch: tc.arch}
			weak := &object{info: info, symbols: []symbol{{name: tc.name, global: true, weak: true}}}
			strong := &object{info: info, symbols: []symbol{{name: tc.name, global: true}}}
			var provider uintptr
			im := &image{base: 0x10000000, mem: make([]byte, 128), resolved: map[string]uintptr{}, got: map[uintptr]uintptr{}, gotStart: 64, gotNext: 64, stubStart: 128, pointerSize: 8}
			if tc.arch == "386" {
				im.pointerSize = 4
			}
			im.external = func(name string) uintptr {
				if name != "optional" {
					t.Fatalf("unexpected provider lookup %q", name)
				}
				return provider
			}
			indirect := strings.HasPrefix(tc.name, "__imp_")
			want := uintptr(0)
			if indirect {
				want = im.base + 64
			}
			for i := 0; i < 3; i++ {
				p, err := im.symbol(weak, 0, false)
				if err != nil || p != want {
					t.Fatalf("missing weak reference: %#x, %v; want %#x", p, err, want)
				}
				if _, err := im.symbol(strong, 0, false); err == nil {
					t.Fatal("a missing weak reference satisfied a later strong reference")
				}
			}
			if indirect && (len(im.got) != 1 || im.gotNext != 64+im.pointerSize || le.Uint64(im.mem[64:]) != 0) {
				t.Fatal("missing weak import slots were not deduplicated at zero")
			}
			provider = 0x22334455
			p, err := im.symbol(strong, 0, false)
			want = provider
			if indirect {
				want = im.base + 64 + uintptr(im.pointerSize)
				var target uint64
				if im.pointerSize == 4 {
					target = uint64(le.Uint32(im.mem[64+im.pointerSize:]))
				} else {
					target = le.Uint64(im.mem[64+im.pointerSize:])
				}
				if target != uint64(provider) {
					t.Fatalf("provided import slot target: %#x", target)
				}
			}
			if err != nil || p != want {
				t.Fatalf("provided strong reference: %#x, %v; want %#x", p, err, want)
			}
			// Successful resolutions still share their cached address or slot.
			provider = 0
			for _, object := range []*object{weak, strong} {
				if cached, err := im.symbol(object, 0, false); err != nil || cached != p {
					t.Fatalf("successful resolution was not cached: %#x, %v", cached, err)
				}
			}
		})
	}
}

func TestNativeELFMixedWeakStrongReferences(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ELF execution requires Linux")
	}
	needNative(t)
	dir := t.TempDir()
	weak := compile(t, "testdata/elf_arm_weak_call.c", filepath.Join(dir, "weak.o"))
	strong := compile(t, "testdata/elf_weak_cache_strong.c", filepath.Join(dir, "strong.o"))
	provider := compile(t, "testdata/elf_arm_weak_provider.c", filepath.Join(dir, "provider.o"))
	providers := filepath.Join(dir, "provider.a")
	command(t, "ar", "rcs", providers, provider)
	consumers := filepath.Join(dir, "consumers.a")
	command(t, "ar", "rcs", consumers, weak, strong)
	for _, tc := range []struct {
		name   string
		inputs []string
		retry  string
	}{
		{"weak_first_object_retry", []string{weak, strong}, provider},
		{"strong_first_object_retry", []string{strong, weak}, provider},
		{"weak_first_archive_retry", []string{weak, strong}, providers},
		{"strong_first_archive_retry", []string{strong, weak}, providers},
		{"consumer_archive_roots", []string{consumers}, providers},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(Options{})
			defer s.Close()
			load(t, s, tc.inputs...)
			if err := s.Link("weak_call", "strong_call"); err == nil || !strings.Contains(err.Error(), "optional_event") {
				t.Fatalf("mixed references must reject the missing strong definition: %v", err)
			}
			if s.image != nil || s.failedImage != nil {
				t.Fatal("an unresolved strong reference published or initialized an image")
			}
			load(t, s, tc.retry)
			if err := s.Link("weak_call", "strong_call"); err != nil {
				t.Fatal(err)
			}
			call(t, s, "weak_call", 20, 22, 42)
			call(t, s, "strong_call", 20, 22, 42)
			call(t, s, "total", 0, 0, 84)
			if len(s.image.objects) != 3 {
				t.Fatalf("retry selected %d objects; want both consumers and their provider", len(s.image.objects))
			}
		})
	}
}
