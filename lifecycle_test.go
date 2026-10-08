package dylib

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	examplecall "github.com/cpunion/dylib-go/examples/call"
)

func lifecycleObserver(t *testing.T, dir string) (string, *Session, func(int32, int32) (int32, error)) {
	t.Helper()
	name, flag := "observer.so", "-shared"
	if runtime.GOOS == "darwin" {
		name, flag = "observer.dylib", "-dynamiclib"
	}
	if runtime.GOOS == "windows" {
		name = "observer.dll"
	}
	path := filepath.Join(dir, name)
	args := []string{flag, "-O0", "-fno-stack-protector", "testdata/lifecycle_host.c", "-o", path}
	if runtime.GOOS != "windows" {
		args = append(args, "-fPIC")
	}
	if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
		args = append(args, "--target=x86_64-apple-macosx11")
	}
	command(t, compiler(), args...)
	observer := New(Options{})
	load(t, observer, path)
	t.Cleanup(func() { observer.Close() })
	symbol, err := observer.Resolve("recorded_event")
	if err != nil {
		t.Fatal(err)
	}
	read := func(a, b int32) (value int32, err error) {
		err = symbol.WithAddress(func(address uintptr) error {
			value, err = examplecall.Int32(address, a, b)
			return err
		})
		return
	}
	return path, observer, read
}

func expectEvents(t *testing.T, read func(int32, int32) (int32, error), want ...int32) {
	t.Helper()
	count, err := read(-1, 0)
	if err != nil || count != int32(len(want)) {
		t.Fatalf("event count: %d %v, want %v", count, err, want)
	}
	for i, expected := range want {
		value, err := read(int32(i), 0)
		if err != nil || value != expected {
			t.Fatalf("event %d: %d %v, want %v", i, value, err, want)
		}
	}
}

func TestNativeObjectConstructionAndExitRegistration(t *testing.T) {
	needNative(t)
	dir := t.TempDir()
	consumer := compile(t, "testdata/lifecycle_consumer.cpp", filepath.Join(dir, "consumer.o"), "-std=c++17", "-fno-exceptions", "-fno-rtti")
	provider := compile(t, "testdata/lifecycle_provider.cpp", filepath.Join(dir, "provider.o"), "-std=c++17", "-fno-exceptions", "-fno-rtti")
	unused := compile(t, "testdata/lifecycle_unused.cpp", filepath.Join(dir, "unused.o"), "-std=c++17", "-fno-exceptions", "-fno-rtti")
	archive := filepath.Join(dir, "providers.a")
	command(t, "ar", "rcs", archive, unused, provider)
	for i, inputs := range [][]string{{consumer, provider}, {provider, consumer}, {consumer, archive}} {
		t.Run([]string{"consumer-first", "provider-first", "archive"}[i], func(t *testing.T) {
			host, _, read := lifecycleObserver(t, t.TempDir())
			s := New(Options{})
			defer s.Close()
			load(t, s, host)
			load(t, s, inputs...)
			expectEvents(t, read) // Load stages raw objects without executing them.
			if err := s.Link("initialized_add"); err != nil {
				t.Fatal(err)
			}
			expectEvents(t, read, 1, 2) // Provider first, independent of load order.
			call(t, s, "initialized_add", 0, 0, 42)
			call(t, s, "register_later", 20, 22, 42)
			if err := s.Link("initialized_add"); err != nil {
				t.Fatal(err)
			}
			expectEvents(t, read, 1, 2)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			expectEvents(t, read, 1, 2, 7, 8, 9) // Reverse registrations, before unmapping.
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			expectEvents(t, read, 1, 2, 7, 8, 9)
		})
	}
}

func TestNativeLifecycleArrays(t *testing.T) {
	needNative(t)
	dir := t.TempDir()
	host, _, read := lifecycleObserver(t, dir)
	obj := compile(t, "testdata/lifecycle_arrays.c", filepath.Join(dir, "arrays.o"))
	s := New(Options{})
	defer s.Close()
	load(t, s, host, obj)
	call(t, s, "array_root", 20, 22, 42)
	expectArrayEvents(t, read, false)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectArrayEvents(t, read, true)
}

func TestNativeInitializationRollbackAndRetry(t *testing.T) {
	needNative(t)
	dir := t.TempDir()
	host, _, read := lifecycleObserver(t, dir)
	obj := compile(t, "testdata/lifecycle_arrays.c", filepath.Join(dir, "arrays.o"))
	s := New(Options{})
	defer s.Close()
	load(t, s, obj)
	if err := s.Link("array_root"); err == nil {
		t.Fatal("missing constructor dependency accepted")
	}
	expectEvents(t, read)
	load(t, s, host)
	if err := s.Link("absent_root"); err == nil {
		t.Fatal("missing root accepted")
	}
	expectEvents(t, read) // Even root validation precedes native initialization.
	if err := s.Link("array_root"); err != nil {
		t.Fatal(err)
	}
	expectArrayEvents(t, read, false)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectArrayEvents(t, read, true)
}

func expectArrayEvents(t *testing.T, read func(int32, int32) (int32, error), closed bool) {
	t.Helper()
	var want []int32
	if runtime.GOOS == "linux" {
		want = append(want, 0)
	}
	want = append(want, 3, 4)
	if closed {
		want = append(want, 5, 6)
	}
	expectEvents(t, read, want...)
}

func TestLifecycleMetadataAcrossTargets(t *testing.T) {
	for _, triple := range []string{"x86_64-linux-gnu", "aarch64-linux-gnu", "i686-linux-gnu", "x86_64-apple-macosx11", "arm64-apple-macosx11", "x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(triple, func(t *testing.T) {
			obj := compile(t, "testdata/lifecycle_arrays.c", filepath.Join(t.TempDir(), "arrays.o"), "--target="+triple)
			data, err := readFile(obj)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(obj, data)
			if err != nil || len(f.info.Unsupported) != 0 {
				t.Fatalf("metadata: %v %v", f, err)
			}
			var init, fini uint64
			for _, s := range f.obj.sections {
				if s == nil {
					continue
				}
				if s.lifecycle == lifecycleInit {
					init += s.size
				}
				if s.lifecycle == lifecycleFini {
					fini += s.size
				}
			}
			if init != uint64(f.info.Bits/4) || fini != init {
				t.Fatalf("arrays: init=%d fini=%d bits=%d", init, fini, f.info.Bits)
			}
		})
	}
}

func TestLifecycleArrayValidation(t *testing.T) {
	for _, sec := range []*section{{name: ".init_array", size: 7}, {name: ".init_array", size: 8, exec: true}, {name: ".init_array.bad", size: 8}, {name: ".fini_array.65536", size: 8}} {
		if err := configureLifecycle(sec, 64); err == nil {
			t.Fatalf("invalid array accepted: %+v", sec)
		}
	}
	for _, bits := range []int{32, 64} {
		mem := make([]byte, 64)
		binary.LittleEndian.PutUint32(mem, 0x1000) // Data section, not executable.
		sec := &section{name: ".init_array", lifecycle: lifecycleInit, size: uint64(bits / 8)}
		o := &object{info: Info{Format: "ELF", Bits: bits}, sections: []*section{nil, sec}}
		im := &image{mem: mem, base: 0x1000, pointerSize: uint64(bits / 8), objects: []*object{o}}
		if err := im.prepareLifecycle(); err == nil {
			t.Fatal("non-executable initializer accepted")
		}
		clear(mem)
		if err := im.prepareLifecycle(); err != nil {
			t.Fatalf("null entry: %v", err)
		}
	}
}

func TestLifecycleDependencyCycles(t *testing.T) {
	a := &object{symbols: []symbol{{name: "a", global: true, section: 1}, {name: "b", global: true}}}
	b := &object{symbols: []symbol{{name: "b", global: true, section: 1}, {name: "a", global: true}}}
	c := &object{symbols: []symbol{{name: "c", global: true, section: 1}, {name: "a", global: true}}}
	for _, o := range []*object{a, b, c} {
		o.relocs = []relocation{{symbol: 1, pair: -1}}
	}
	defs := map[string]definition{"a": {a, 0}, "b": {b, 0}, "c": {c, 0}}
	order, err := (&image{objects: []*object{c, a, b}, defs: defs}).lifecycleObjectOrder()
	if err != nil || order[a] != 0 || order[b] != 1 || order[c] != 2 {
		t.Fatalf("cycle order: %v %v", order, err)
	}
}

// Inspect must still reject lifecycle forms whose execution contracts differ.
func TestRejectUnsupportedLifecycleForms(t *testing.T) {
	for _, tc := range []struct{ triple, source string }{
		{"x86_64-linux-gnu", ".section .init,\"ax\",@progbits\n.byte 0\n"},
		{"x86_64-pc-windows-msvc", ".section .CRT$XLA,\"dr\"\n.quad 0\n"},
	} {
		file := filepath.Join(t.TempDir(), "unsupported.s")
		if err := os.WriteFile(file, []byte(tc.source), 0600); err != nil {
			t.Fatal(err)
		}
		out := file + ".o"
		command(t, compiler(), "--target="+tc.triple, "-c", file, "-o", out)
		i, err := Inspect(out)
		if err != nil || len(i.Unsupported) == 0 {
			t.Fatalf("unsupported metadata: %+v %v", i, err)
		}
	}
}

func TestNativeScopedAndRecursiveFinalization(t *testing.T) {
	needNative(t)
	dir := t.TempDir()
	host, _, read := lifecycleObserver(t, dir)
	obj := compile(t, "testdata/lifecycle_exits.c", filepath.Join(dir, "exits.o"))
	a, b := New(Options{}), New(Options{})
	defer a.Close()
	defer b.Close()
	for _, s := range []*Session{a, b} {
		load(t, s, host, obj)
		call(t, s, "register_selective", 20, 22, 42)
		call(t, s, "dso_self", 20, 22, 42)
	}
	call(t, a, "finalize_tag", 20, 22, 42)
	expectEvents(t, read, 11)
	call(t, a, "finalize_all", 20, 22, 42)
	expectEvents(t, read, 11, 13, 12, 14)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 11, 13, 12, 14)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 11, 13, 12, 14, 13, 12, 14, 11)
}
