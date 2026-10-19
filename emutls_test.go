package dylib

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cpunion/dylib-go/internal/native"
)

func TestEmulatedTLSAcrossTargets(t *testing.T) {
	for _, target := range []string{"x86_64-unknown-linux-gnu", "aarch64-unknown-linux-gnu", "i686-unknown-linux-gnu", "x86_64-apple-macosx11", "arm64-apple-macosx11", "x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc", "x86_64-w64-windows-gnu", "aarch64-w64-windows-gnu", "i686-w64-windows-gnu"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			path := compile(t, "testdata/emutls_consumer.c", filepath.Join(dir, "consumer.o"), "--target="+target, "-ffreestanding", "-femulated-tls")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(path, data)
			if err != nil {
				t.Fatal(err)
			}
			if len(f.info.Unsupported) != 0 {
				t.Fatal(f.info.Unsupported)
			}
			needed, err := checkEmulatedTLS([]*object{f.obj}, true)
			if err != nil || !needed {
				t.Fatal(needed, err)
			}
			if _, err := checkEmulatedTLS([]*object{f.obj}, false); err == nil {
				t.Fatal("missing opt-in accepted")
			}
			count, helper := 0, false
			for _, s := range f.obj.symbols {
				if strings.HasPrefix(s.name, emulatedTLSPrefix) && s.section > 0 {
					count++
				}
				helper = helper || s.name == emulatedTLSHelper && s.section == 0
			}
			if count != 2 || !helper {
				t.Fatalf("descriptor names/layout lost: %d %v", count, helper)
			}
			path = compile(t, "testdata/emutls_destructor.cpp", filepath.Join(dir, "destructor.o"), "--target="+target, "-ffreestanding", "-std=c++17", "-femulated-tls", "-fno-exceptions", "-fno-rtti")
			data, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err = parse(path, data)
			if err != nil {
				t.Fatal(err)
			}
			_, err = checkEmulatedTLS([]*object{f.obj}, true)
			if strings.HasSuffix(target, "-msvc") {
				if err == nil || !strings.Contains(err.Error(), "thread destructors") {
					t.Fatal("MSVC dynamic TLS runtime accepted", err)
				}
			} else if err != nil {
				t.Fatal("image-owned thread destructor registration rejected", err)
			}
		})
	}
}

func tlsValidationImage(word uint64) *image {
	o := &object{sections: []*section{nil, {name: "data", size: 512, write: true}}, symbols: []symbol{{name: "__emutls_v.x", section: 1, size: 4 * word, global: true}}}
	im := &image{base: 0x10000, mem: make([]byte, 512), objects: []*object{o}, pointerSize: word, defs: map[string]definition{"__emutls_v.x": {o, 0}}}
	putTLSWord(im, 0, 4)
	putTLSWord(im, word, 64)
	putTLSWord(im, 3*word, uint64(im.base)+256)
	return im
}
func putTLSWord(im *image, offset, value uint64) {
	if im.pointerSize == 4 {
		binary.LittleEndian.PutUint32(im.mem[offset:], uint32(value))
	} else {
		binary.LittleEndian.PutUint64(im.mem[offset:], value)
	}
}

func TestEmulatedTLSDescriptorValidation(t *testing.T) {
	for _, word := range []uint64{4, 8} {
		for _, tc := range []struct {
			name   string
			change func(*image)
		}{
			{"zero_size", func(im *image) { putTLSWord(im, 0, 0) }},
			{"huge_size", func(im *image) { putTLSWord(im, 0, maxImage+1) }},
			{"zero_alignment", func(im *image) { putTLSWord(im, word, 0) }},
			{"non_power_alignment", func(im *image) { putTLSWord(im, word, 3) }},
			{"huge_alignment", func(im *image) { putTLSWord(im, word, maxImage*2) }},
			{"object_in_use", func(im *image) { putTLSWord(im, 2*word, 1) }},
			{"foreign_template", func(im *image) { putTLSWord(im, 3*word, uint64(im.base)-1) }},
			{"truncated_template", func(im *image) { putTLSWord(im, 3*word, uint64(im.base)+511) }},
			{"readonly_descriptor", func(im *image) { im.objects[0].sections[1].write = false }},
			{"executable_descriptor", func(im *image) { im.objects[0].sections[1].exec = true }},
			{"truncated_descriptor", func(im *image) { im.objects[0].sections[1].size = 4*word - 1 }},
			{"wrong_symbol_size", func(im *image) { im.objects[0].symbols[0].size = 1 }},
			{"external_descriptor", func(im *image) { im.objects[0].symbols[0].section = -1 }},
			{"budget", func(im *image) { putTLSWord(im, 0, maxImage); putTLSWord(im, 3*word, 0) }},
			{"overlap", func(im *image) {
				im.objects[0].symbols = append(im.objects[0].symbols, symbol{name: "__emutls_v.y", section: 1, value: 3 * word, size: 4 * word})
				putTLSWord(im, 4*word, 64)
			}},
		} {
			t.Run(strconv.FormatUint(word, 10)+"/"+tc.name, func(t *testing.T) {
				im := tlsValidationImage(word)
				if got, err := im.emulatedTLSDescriptors(); err != nil || len(got) != 1 {
					t.Fatal(got, err)
				}
				tc.change(im)
				if _, err := im.emulatedTLSDescriptors(); err == nil {
					t.Fatal("invalid descriptor accepted")
				}
			})
		}
	}
}

func TestEmulatedTLSReservedRuntime(t *testing.T) {
	s := New(Options{EmulatedTLS: true})
	defer s.Close()
	for _, name := range []string{emulatedTLSHelper, "__imp_" + emulatedTLSHelper, "__emutls_v.x"} {
		if err := s.Define(name, 1); err == nil {
			t.Fatal("external runtime override accepted", name)
		}
	}
	for _, name := range []string{"__cxa_thread_atexit", "__cxa_thread_atexit_impl", "_tlv_atexit", "__tlv_atexit", "_tlregdtor", "__tlregdtor", "__dyn_tls_on_demand_init"} {
		if err := s.Define(name, 1); err == nil {
			t.Fatal("runtime override accepted", name)
		}
		o := &object{info: Info{OS: "darwin"}, symbols: []symbol{{name: "__emutls_v.x", section: 1}, {name: name}}}
		_, err := checkEmulatedTLS([]*object{o}, true)
		if emulatedTLSThreadHelper(name) {
			if err != nil {
				t.Fatal(name, err)
			}
			o.symbols[1].section = 1
			if _, err := checkEmulatedTLS([]*object{o}, true); err == nil {
				t.Fatal("defined thread helper accepted", name)
			}
			o.symbols[1].section = 0
			o.symbols[1].imported = &coffImportSymbol{}
			if _, err := checkEmulatedTLS([]*object{o}, true); err == nil {
				t.Fatal("imported thread runtime accepted", name)
			}
		} else if err == nil || !strings.Contains(err.Error(), "thread destructors") {
			t.Fatal(name, err)
		}
	}
}

func TestEmulatedTLSRuntimeWithoutDescriptors(t *testing.T) {
	for _, word := range []uint64{4, 8} {
		o := &object{symbols: []symbol{{name: "__cxa_thread_atexit"}}}
		im := &image{pointerSize: word, objects: []*object{o}}
		if _, err := checkEmulatedTLS(im.objects, false); err == nil {
			t.Fatal("unowned runtime fallback accepted")
		}
		if needed, err := checkEmulatedTLS(im.objects, true); err != nil || !needed {
			t.Fatal(needed, err)
		}
		if descriptors, err := im.emulatedTLSDescriptors(); err != nil || len(descriptors) != 0 {
			t.Fatal(descriptors, err)
		}
		o.symbols = append(o.symbols, symbol{name: emulatedTLSHelper})
		if _, err := im.emulatedTLSDescriptors(); err == nil {
			t.Fatal("address helper accepted without descriptors")
		}
	}
}

func TestNativeEmulatedTLSLifecycle(t *testing.T) {
	needNative(t)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	dir := t.TempDir()
	host, _, read := lifecycleObserver(t, dir)
	obj := compile(t, "testdata/emutls_lifecycle.c", filepath.Join(dir, "lifecycle.o"), "-femulated-tls")
	s := New(Options{EmulatedTLS: true})
	defer s.Close()
	load(t, s, host, obj)
	expectEvents(t, read)
	if err := s.Link("missing_root"); err == nil {
		t.Fatal("invalid root accepted")
	}
	expectEvents(t, read)
	call(t, s, "tls_lifecycle", 20, 22, 50)
	expectEvents(t, read, 7)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 7, 9, 10) // Exit hook, then static finalizer, with TLS still alive.
}

func TestNativeEmulatedTLSRetirement(t *testing.T) {
	needNative(t)
	host := nativeThreadLibrary(t, "testdata/emutls_threads.c")
	start, err := host.Resolve("tls_threads_pause")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var inputs []string
	for _, part := range []string{"consumer", "provider"} {
		inputs = append(inputs, compile(t, "testdata/emutls_"+part+".c", filepath.Join(dir, part+".o"), "-femulated-tls"))
	}
	for _, mode := range []string{"owner_before_join", "concurrent_exit"} {
		t.Run(mode, func(t *testing.T) {
			for generation := 0; generation < 8; generation++ {
				s := New(Options{EmulatedTLS: true})
				defer s.Close()
				load(t, s, inputs...)
				step, err := s.Resolve("tls_step")
				if err != nil {
					t.Fatal(err)
				}
				lease, err := step.Acquire()
				if err != nil {
					t.Fatal(err)
				}
				address, err := lease.Address()
				if err != nil {
					t.Fatal(err)
				}
				if err := start.WithAddress(func(run uintptr) error { native.CallPointer(run, address); return nil }); err != nil {
					t.Fatal(err)
				}
				// On failure join foreign users before releasing their code lease.
				joined := false
				defer func() {
					defer lease.Close()
					if !joined {
						call(t, host, "tls_threads_release", 0, 0, 0)
					}
				}()
				deadline := time.Now().Add(5 * time.Second)
				for {
					ready, err := callInt32(host, "tls_threads_ready", 0, 0)
					if err != nil {
						t.Fatal(err)
					}
					if ready == 4 {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("native TLS workers did not reach retirement barrier")
					}
					time.Sleep(time.Millisecond)
				}
				if n := s.image.emulatedTLS.LiveValues(); n != 12 {
					t.Fatal("live native thread values", n)
				}
				tls := s.image.emulatedTLS
				closed := make(chan error, 1)
				go func() { closed <- s.Close() }()
				waitSessionRetired(t, s)
				select {
				case <-closed:
					t.Fatal("TLS owner ignored retained code lease")
				default:
				}
				lease.Close()
				if mode == "concurrent_exit" {
					call(t, host, "tls_threads_release", 0, 0, 0)
					joined = true
				}
				if err := <-closed; err != nil {
					t.Fatal(err)
				}
				if tls.LiveValues() != 0 {
					t.Fatal("retired owner retained thread values")
				}
				if !joined {
					call(t, host, "tls_threads_release", 0, 0, 0)
					joined = true
				}
			}
		})
	}
}

func TestNativeEmulatedTLSInlineAndIsolation(t *testing.T) {
	needNative(t)
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			dir := t.TempDir()
			a := compile(t, "testdata/emutls_inline_a.cpp", filepath.Join(dir, "a.o"), "-femulated-tls", "-std=c++17", "-fno-exceptions", "-fno-rtti", opt)
			b := compile(t, "testdata/emutls_inline_b.cpp", filepath.Join(dir, "b.o"), "-femulated-tls", "-std=c++17", "-fno-exceptions", "-fno-rtti", opt)
			archive := filepath.Join(dir, "inline.a")
			command(t, "ar", "rcs", archive, a, b)
			for _, kind := range []string{"objects", "reversed", "archive"} {
				t.Run(kind, func(t *testing.T) {
					runtime.LockOSThread()
					defer runtime.UnlockOSThread()
					inputs := []string{a, b}
					if kind == "reversed" {
						inputs = []string{b, a}
					}
					if kind == "archive" {
						inputs = []string{archive}
					}
					var owners [2]*Session
					for i := range owners {
						owners[i] = New(Options{EmulatedTLS: true})
						defer owners[i].Close()
						load(t, owners[i], inputs...)
						if err := owners[i].Link("tls_inline_a", "tls_inline_b"); err != nil {
							t.Fatal(err)
						}
						call(t, owners[i], "tls_inline_a", 20, 22, 50)
						call(t, owners[i], "tls_inline_b", 20, 22, 51)
						if n := owners[i].image.emulatedTLS.LiveValues(); n != 1 {
							t.Fatal("weak TLS descriptor not coalesced", n)
						}
					}
					if err := owners[0].Close(); err != nil {
						t.Fatal(err)
					}
					call(t, owners[1], "tls_inline_a", 20, 22, 52)
				})
			}
		})
	}
}

func TestNativeEmulatedTLS(t *testing.T) {
	needNative(t)
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			dir := t.TempDir()
			consumer := compile(t, "testdata/emutls_consumer.c", filepath.Join(dir, "consumer.o"), "-femulated-tls", opt)
			provider := compile(t, "testdata/emutls_provider.c", filepath.Join(dir, "provider.o"), "-femulated-tls", opt)
			archive := filepath.Join(dir, "provider.a")
			command(t, "ar", "rcs", archive, provider)
			for _, kind := range []string{"object", "archive"} {
				t.Run(kind, func(t *testing.T) {
					runtime.LockOSThread()
					defer runtime.UnlockOSThread()
					input := provider
					if kind == "archive" {
						input = archive
					}
					for generation := 0; generation < 8; generation++ {
						s := New(Options{EmulatedTLS: true})
						load(t, s, consumer)
						// A failed link must leave staged descriptor bytes reusable.
						if err := s.Link("tls_step"); err == nil {
							t.Fatal("missing TLS provider accepted")
						}
						load(t, s, input)
						if err := s.Link("tls_step"); err != nil {
							t.Fatal(err)
						}
						for i := int32(0); i < 100; i++ {
							call(t, s, "tls_step", 20, 22, 49+i)
						}
						if got := s.image.emulatedTLS.LiveValues(); got != 3 {
							t.Fatal("per-thread TLS values", got)
						}
						tls := s.image.emulatedTLS
						if err := s.Close(); err != nil {
							t.Fatal(err)
						}
						if tls.LiveValues() != 0 {
							t.Fatal("owner close retained TLS values")
						}
					}
				})
			}
		})
	}
}

func TestNativeEmulatedTLSForeignThreads(t *testing.T) {
	needNative(t)
	host := nativeThreadLibrary(t, "testdata/emutls_threads.c")
	runner, err := host.Resolve("tls_threads")
	if err != nil {
		t.Fatal(err)
	}
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			dir := t.TempDir()
			s := New(Options{EmulatedTLS: true})
			defer s.Close()
			for _, part := range []string{"consumer", "provider"} {
				load(t, s, compile(t, "testdata/emutls_"+part+".c", filepath.Join(dir, part+".o"), "-femulated-tls", opt))
			}
			step, err := s.Resolve("tls_step")
			if err != nil {
				t.Fatal(err)
			}
			for round := 0; round < 8; round++ {
				err = runner.WithAddress(func(run uintptr) error {
					return step.WithAddress(func(fn uintptr) error { native.CallPointer(run, fn); return nil })
				})
				if err != nil {
					t.Fatal(err)
				}
				call(t, host, "tls_thread_result", 0, 0, 0)
				if live := s.image.emulatedTLS.LiveValues(); live != 0 {
					t.Fatal("thread exit retained TLS values", live)
				}
			}
		})
	}
}
