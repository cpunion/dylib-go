package dylib

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestLegacyELFLifecycleMetadata(t *testing.T) {
	for _, triple := range []string{"x86_64-linux-gnu", "aarch64-linux-gnu", "i686-linux-gnu"} {
		t.Run(triple, func(t *testing.T) {
			path := compile(t, "testdata/lifecycle_legacy.c", filepath.Join(t.TempDir(), "legacy.o"), "--target="+triple, "-fno-use-init-array")
			b, err := readFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(path, b)
			if err != nil || len(f.info.Unsupported) != 0 {
				t.Fatalf("legacy metadata: %v, %v", f, err)
			}
			counts := make(map[lifecycleKind]uint64)
			priorities := make(map[lifecycleKind][]uint32)
			for _, s := range f.obj.sections {
				if s != nil && s.legacyLifecycle {
					counts[s.lifecycle] += s.size / uint64(f.info.Bits/8)
					priorities[s.lifecycle] = append(priorities[s.lifecycle], s.priority)
				}
			}
			for _, kind := range []lifecycleKind{lifecycleInit, lifecycleFini} {
				if counts[kind] != 7 || len(priorities[kind]) != 3 {
					t.Fatalf("legacy tables: counts=%v priorities=%v", counts, priorities)
				}
				for _, want := range []uint32{101, 201, 65535} {
					found := false
					for _, priority := range priorities[kind] {
						found = found || priority == want
					}
					if !found {
						t.Fatalf("missing priority %d: %v", want, priorities[kind])
					}
				}
			}
		})
	}
}

func TestLegacyELFLifecycleValidation(t *testing.T) {
	for _, name := range []string{".ctors.bad", ".ctors.", ".dtors.65536"} {
		if err := configureLifecycle(&section{name: name, size: 8, legacyLifecycle: true}, 64); err == nil {
			t.Fatalf("invalid legacy priority accepted: %s", name)
		}
	}
	for _, bits := range []int{32, 64} {
		width := uint64(bits / 8)
		mem := make([]byte, 256)
		write := func(offset uint64, values ...uint64) {
			for i, value := range values {
				b := mem[offset+uint64(i)*width:]
				if bits == 32 {
					binary.LittleEndian.PutUint32(b, uint32(value))
				} else {
					binary.LittleEndian.PutUint64(b, value)
				}
			}
		}
		init := &section{name: ".ctors", lifecycle: lifecycleInit, legacyLifecycle: true, size: 5 * width}
		fini := &section{name: ".dtors", lifecycle: lifecycleFini, legacyLifecycle: true, size: 5 * width, offset: 5 * width}
		code := &section{name: ".text", exec: true, size: 64, offset: 128}
		for _, s := range []*section{init, fini} {
			if err := configureLifecycle(s, bits); err != nil {
				t.Fatal(err)
			}
		}
		write(0, ^uint64(0), 0x1080, 0, 0x1084, 0)
		write(fini.offset, ^uint64(0), 0x1088, 0, 0x108c, 0)
		o := &object{info: Info{Format: "ELF", Bits: bits}, sections: []*section{nil, init, fini, code}}
		im := &image{mem: mem, base: 0x1000, pointerSize: width, objects: []*object{o}}
		if err := im.prepareLifecycle(); err != nil || !reflect.DeepEqual(im.initializers, []uintptr{0x1084, 0x1080}) || !reflect.DeepEqual(im.finalizers, []uintptr{0x1088, 0x108c}) {
			t.Fatalf("%d-bit legacy order: init=%v fini=%v err=%v", bits, im.initializers, im.finalizers, err)
		}
		// -1 is a sentinel only in legacy tables; normal arrays must reject it.
		init.name, init.legacyLifecycle = ".init_array", false
		if err := im.prepareLifecycle(); err == nil {
			t.Fatal("modern array accepted a legacy sentinel")
		}
		init.name, init.legacyLifecycle = ".ctors", true
		write(0, 1) // Count-prefixed GNU linker sets are a different contract.
		if err := im.prepareLifecycle(); err == nil {
			t.Fatal("legacy table accepted a count or non-code pointer")
		}
	}
	for _, tc := range []struct {
		name, declaration string
	}{
		{".ctors", "char table[7] = {0};"},
		{".ctors_data", "char table[1] = {0};"},
	} {
		path := filepath.Join(t.TempDir(), "table.c")
		src := "__attribute__((used, section(\"" + tc.name + "\"))) " + tc.declaration
		if err := os.WriteFile(path, []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
		out := compile(t, path, path+".o", "--target=x86_64-linux-gnu")
		info, err := Inspect(out)
		if tc.name == ".ctors_data" {
			if err != nil || len(info.Unsupported) != 0 {
				t.Fatalf("ordinary section treated as a table: %+v, %v", info, err)
			}
		} else if err == nil {
			t.Fatalf("invalid legacy table accepted: %s", tc.name)
		}
	}
	path := filepath.Join(t.TempDir(), "nobits.s")
	if err := os.WriteFile(path, []byte(".section .dtors,\"aw\",@nobits\n.zero 8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command(t, compiler(), "--target=x86_64-linux-gnu", "-c", path, "-o", path+".o")
	if _, err := Inspect(path + ".o"); err == nil || !strings.Contains(err.Error(), "SHT_PROGBITS") {
		t.Fatalf("legacy NOBITS table accepted: %v", err)
	}
}

func TestNativeLegacyELFLifecycle(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ELF execution requires Linux")
	}
	needNative(t)
	dir := t.TempDir()
	obj := compile(t, "testdata/lifecycle_legacy.c", filepath.Join(dir, "legacy.o"), "-fno-use-init-array")
	host, _, read := lifecycleObserver(t, dir)
	s := New(Options{})
	defer s.Close()
	load(t, s, host, obj)
	expectEvents(t, read)
	call(t, s, "legacy_root", 0, 0, 42)
	expectEvents(t, read, 0, 1, 2, 3, 4, 5)
	if err := s.Link("legacy_root"); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 0, 1, 2, 3, 4, 5)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
}

func TestNativeLegacyELFValidationBeforeInitialization(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ELF execution requires Linux")
	}
	needNative(t)
	dir := t.TempDir()
	obj := compile(t, "testdata/lifecycle_legacy.c", filepath.Join(dir, "bad.o"), "-fno-use-init-array", "-DBAD_LEGACY_FINALIZER")
	host, _, read := lifecycleObserver(t, dir)
	s := New(Options{})
	defer s.Close()
	load(t, s, host, obj)
	if err := s.Link("legacy_root"); err == nil || !strings.Contains(err.Error(), "outside executable") || s.image != nil {
		t.Fatalf("invalid finalizer accepted/published: %v", err)
	}
	expectEvents(t, read)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read)
}

func TestNativeLegacyELFDependencyAndArchive(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ELF execution requires Linux")
	}
	needNative(t)
	dir := t.TempDir()
	flags := []string{"-std=c++17", "-fno-exceptions", "-fno-rtti", "-fno-use-init-array"}
	consumer := compile(t, "testdata/lifecycle_consumer.cpp", filepath.Join(dir, "consumer.o"), flags...)
	provider := compile(t, "testdata/lifecycle_provider.cpp", filepath.Join(dir, "provider.o"), flags...)
	unused := compile(t, "testdata/lifecycle_unused.cpp", filepath.Join(dir, "unused.o"), flags...)
	archive := filepath.Join(dir, "providers.a")
	command(t, "ar", "rcs", archive, unused, provider)
	for i, inputs := range [][]string{{consumer, provider}, {provider, consumer}, {consumer, archive}} {
		t.Run([]string{"consumer-first", "provider-first", "archive"}[i], func(t *testing.T) {
			host, _, read := lifecycleObserver(t, t.TempDir())
			s := New(Options{})
			defer s.Close()
			load(t, s, inputs...)
			if err := s.Link("initialized_add"); err == nil || !strings.Contains(err.Error(), "record_event") {
				t.Fatalf("missing initializer dependency: %v", err)
			}
			expectEvents(t, read)
			load(t, s, host)
			call(t, s, "initialized_add", 0, 0, 42)
			expectEvents(t, read, 1, 2)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			expectEvents(t, read, 1, 2, 8, 9)
		})
	}
}
