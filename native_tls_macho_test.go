package dylib

import (
	"bytes"
	"debug/macho"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ramblelabs/dylib-go/internal/native"
)

func TestMachONativeTLSEntryEncoding(t *testing.T) {
	for _, tc := range []struct{ arch, target string }{{"amd64", "x86_64-apple-macosx11"}, {"arm64", "arm64-apple-macosx11"}} {
		t.Run(tc.arch, func(t *testing.T) {
			path := compile(t, "testdata/macho_tls_entry.S", filepath.Join(t.TempDir(), "entry.o"), "--target="+tc.target)
			f, err := macho.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			want, err := f.Section("__text").Data()
			if err != nil {
				t.Fatal(err)
			}
			got, err := machoTLSEntry(tc.arch, 0x12345678)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("entry differs from the independent assembler: %v\ngot %x\nwant%x", err, got, want)
			}
		})
	}
	if _, err := machoTLSEntry("arm64", 0); err == nil {
		t.Fatal("missing native allocator accepted")
	}
	if _, err := machoTLSEntry("386", 1); err == nil {
		t.Fatal("unsupported TLV architecture accepted")
	}
}

func TestNativeMachOTLSRegisters(t *testing.T) {
	needMachONativeTLS(t)
	s := New(Options{NativeTLS: true})
	defer s.Close()
	dir := t.TempDir()
	load(t, s, compile(t, "testdata/macho_tls_registers.S", filepath.Join(dir, "registers.o")), compile(t, "testdata/emutls_provider.c", filepath.Join(dir, "provider.o"), "-fno-emulated-tls"))
	for i := 0; i < 100; i++ {
		call(t, s, "tls_registers", 20, 22, 42)
	}
}

func TestMachONativeTLSAcrossTargets(t *testing.T) {
	for _, target := range []string{"x86_64-apple-macosx11", "arm64-apple-macosx11"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			var objects []*object
			for _, part := range []string{"consumer", "provider"} {
				path := compile(t, "testdata/emutls_"+part+".c", filepath.Join(dir, part+".o"), "--target="+target, "-ffreestanding", "-fno-emulated-tls")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				f, err := parse(path, data)
				if err != nil || len(f.info.Unsupported) != 0 {
					t.Fatal(f, err)
				}
				objects = append(objects, f.obj)
				for _, sec := range f.obj.sections {
					if sec != nil && sec.nativeTLS == 0x12 && len(sec.data) != 0 {
						t.Fatal("native TLS zero fill read object header bytes")
					}
				}
			}
			if needed, err := checkMachOTLS(objects, true); err != nil || !needed {
				t.Fatal(needed, err)
			}
			if _, err := checkMachOTLS(objects, false); err == nil {
				t.Fatal("native TLS accepted without opt-in")
			}
			im := &image{objects: objects, pointerSize: 8, page: 4096}
			if _, err := im.layoutMachOTLS(0); err != nil || im.machoTLS.align != 64 || im.machoTLS.size < 68 {
				t.Fatal(im.machoTLS, err)
			}
		})
	}
}

func needMachONativeTLS(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("native Mach-O TLS execution requires macOS")
	}
	needNative(t)
}

func TestNativeMachOTLS(t *testing.T) {
	needMachONativeTLS(t)
	testNativeTLSModules(t)
}

func testNativeTLSModules(t *testing.T) {
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			dir := t.TempDir()
			consumer := compile(t, "testdata/emutls_consumer.c", filepath.Join(dir, "consumer.o"), "-fno-emulated-tls", opt)
			provider := compile(t, "testdata/emutls_provider.c", filepath.Join(dir, "provider.o"), "-fno-emulated-tls", opt)
			poison := "testdata/macho_tls_unowned.c"
			if runtime.GOOS == "linux" {
				poison = "testdata/elf_tls_unowned.c"
			}
			unowned := compile(t, poison, filepath.Join(dir, "unowned.o"), opt)
			archive := filepath.Join(dir, "provider.a")
			command(t, "ar", "rcs", archive, unowned, provider)
			for _, kind := range []string{"object", "archive"} {
				t.Run(kind, func(t *testing.T) {
					runtime.LockOSThread()
					defer runtime.UnlockOSThread()
					for generation := 0; generation < 3; generation++ {
						s := New(Options{NativeTLS: true})
						defer s.Close()
						load(t, s, consumer)
						if err := s.Link("tls_step"); err == nil || s.image != nil {
							t.Fatal("missing TLS provider accepted:", err)
						}
						input := provider
						if kind == "archive" {
							input = archive
						}
						load(t, s, input)
						for i := int32(0); i < 100; i++ {
							call(t, s, "tls_step", 20, 22, 49+i)
						}
						if _, err := s.Resolve("tls_shared"); err == nil || !strings.Contains(err.Error(), "native TLS") {
							t.Fatal("descriptor exposed as a process-global variable:", err)
						}
						owner := s.image.emulatedTLS
						if n := owner.LiveValues(); n != 1 {
							t.Fatal("expected one native TLS module allocation:", n)
						}
						if err := s.Close(); err != nil || owner.LiveValues() != 0 {
							t.Fatal("native TLS owner leaked:", err)
						}
					}
				})
			}
		})
	}
}

func TestNativeMachOTLSOptIn(t *testing.T) {
	needMachONativeTLS(t)
	dir := t.TempDir()
	s := New(Options{})
	defer s.Close()
	for _, part := range []string{"consumer", "provider"} {
		load(t, s, compile(t, "testdata/emutls_"+part+".c", filepath.Join(dir, part+".o")))
	}
	if err := s.Link("tls_step"); err == nil || !strings.Contains(err.Error(), "Options.NativeTLS") || s.image != nil {
		t.Fatal("native TLS accepted without opt-in:", err)
	}
}

func TestNativeMachOTLSRetainedProvider(t *testing.T) {
	needMachONativeTLS(t)
	testNativeTLSRetainedProvider(t)
}

func testNativeTLSRetainedProvider(t *testing.T, flags ...string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	dir := t.TempDir()
	provider := New(Options{NativeTLS: true})
	defer provider.Close()
	for _, part := range []string{"consumer", "provider"} {
		load(t, provider, compile(t, "testdata/emutls_"+part+".c", filepath.Join(dir, part+".o"), append([]string{"-O2"}, flags...)...))
	}
	step, err := provider.Resolve("tls_step")
	if err != nil {
		t.Fatal(err)
	}
	owner := provider.image.emulatedTLS
	consumer := New(Options{})
	defer consumer.Close()
	if err := consumer.DefineSymbol("tls_step", step); err != nil {
		t.Fatal(err)
	}
	load(t, consumer, compile(t, "testdata/macho_tls_forward.c", filepath.Join(dir, "forward.o"), "-O2"))
	if err := consumer.Link("native_tls_forward"); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- provider.Close() }()
	waitSessionRetired(t, provider)
	for i := int32(0); i < 100; i++ {
		call(t, consumer, "native_tls_forward", 20, 22, 49+i)
	}
	select {
	case <-closed:
		t.Fatal("provider retired while the consumer retained its TLS code")
	default:
	}
	if err := consumer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil || owner.LiveValues() != 0 {
		t.Fatal("retained provider TLS cleanup:", err)
	}
}

func TestNativeMachOTLSForeignThreads(t *testing.T) {
	needMachONativeTLS(t)
	testNativeTLSForeignThreads(t)
}

func testNativeTLSForeignThreads(t *testing.T) {
	host := nativeThreadLibrary(t, "testdata/emutls_threads.c")
	runner, err := host.Resolve("tls_threads")
	if err != nil {
		t.Fatal(err)
	}
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			dir := t.TempDir()
			s := New(Options{NativeTLS: true})
			defer s.Close()
			for _, part := range []string{"consumer", "provider"} {
				load(t, s, compile(t, "testdata/emutls_"+part+".c", filepath.Join(dir, part+".o"), "-fno-emulated-tls", opt))
			}
			step, err := s.Resolve("tls_step")
			if err != nil {
				t.Fatal(err)
			}
			for round := 0; round < 4; round++ {
				if err := runner.WithAddress(func(run uintptr) error {
					return step.WithAddress(func(fn uintptr) error { native.CallPointer(run, fn); return nil })
				}); err != nil {
					t.Fatal(err)
				}
				call(t, host, "tls_thread_result", 0, 0, 0)
				if live := s.image.emulatedTLS.LiveValues(); live != 0 {
					t.Fatal("native thread exit retained TLS values:", live)
				}
			}
		})
	}
}

func TestNativeMachOTLSValuesAndIsolation(t *testing.T) {
	needMachONativeTLS(t)
	testNativeTLSValuesAndIsolation(t)
}

func testNativeTLSValuesAndIsolation(t *testing.T) {
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			dir := t.TempDir()
			values := compile(t, "testdata/macho_tls_values.c", filepath.Join(dir, "values.o"), opt)
			for _, mixed := range []bool{false, true} {
				t.Run(fmt.Sprint("emulated", mixed), func(t *testing.T) {
					runtime.LockOSThread()
					defer runtime.UnlockOSThread()
					var owners [2]*Session
					for n := range owners {
						// i386 O0 aggregate copies can use the retained host memcpy.
						s := New(Options{NativeTLS: true, EmulatedTLS: mixed, ProcessSymbols: true})
						owners[n] = s
						defer s.Close()
						load(t, s, values)
						if mixed {
							for _, part := range []string{"consumer", "provider"} {
								load(t, s, compile(t, "testdata/emutls_"+part+".c", filepath.Join(dir, fmt.Sprintf("emulated-%d-%s.o", n, part)), "-femulated-tls", opt))
							}
						}
						for i := int32(0); i < 20; i++ {
							call(t, s, "native_tls_values", 20, 22, 62+i)
							call(t, s, "native_tls_mixed", 20, 22, 42)
							if mixed {
								call(t, s, "tls_step", 20, 22, 49+i)
							}
						}
						want := 1
						if mixed {
							want = 4
						}
						if got := s.image.emulatedTLS.LiveValues(); got != want {
							t.Fatal("native/emulated owner storage:", got, want)
						}
					}
					if err := owners[0].Close(); err != nil {
						t.Fatal(err)
					}
					call(t, owners[1], "native_tls_values", 20, 22, 82)
				})
			}
		})
	}
}

func TestNativeMachOTLSPinnedGoThreads(t *testing.T) {
	needMachONativeTLS(t)
	testNativeTLSPinnedGoThreads(t)
}

func testNativeTLSPinnedGoThreads(t *testing.T, flags ...string) {
	s := New(Options{NativeTLS: true})
	defer s.Close()
	dir := t.TempDir()
	for _, part := range []string{"consumer", "provider"} {
		load(t, s, compile(t, "testdata/emutls_"+part+".c", filepath.Join(dir, part+".o"), append([]string{"-O2"}, flags...)...))
	}
	step, err := s.Resolve("tls_step")
	if err != nil {
		t.Fatal(err)
	}
	ready, done := make(chan struct{}, 2), make(chan error, 2)
	start, release := make(chan struct{}), make(chan struct{})
	var workers sync.WaitGroup
	defer func() { close(release); workers.Wait() }()
	for n := 0; n < 2; n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ready <- struct{}{}
			<-start
			var failure error
			for i := int32(0); i < 100; i++ {
				got, err := callSymbolInt32(step, 20, 22)
				if err != nil || got != 49+i {
					failure = fmt.Errorf("pinned thread call%d: got%d, %v", i, got, err)
					break
				}
			}
			done <- failure
			<-release
		}()
	}
	<-ready
	<-ready
	close(start)
	for n := 0; n < 2; n++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	owner := s.image.emulatedTLS
	if n := owner.LiveValues(); n != 2 {
		t.Fatal("pinned native OS-thread isolation:", n)
	}
	if err := s.Close(); err != nil || owner.LiveValues() != 0 {
		t.Fatal("retirement left per-thread storage:", err)
	}
}

func TestNativeMachOTLSLifecycle(t *testing.T) {
	needMachONativeTLS(t)
	testNativeTLSLifecycle(t)
}

func testNativeTLSLifecycle(t *testing.T, flags ...string) {
	testNativeTLSLifecycleInput(t, func(dir string) string {
		return compile(t, "testdata/emutls_lifecycle.c", filepath.Join(dir, "lifecycle.o"), append([]string{"-fno-emulated-tls"}, flags...)...)
	})
}

func testNativeTLSLifecycleInput(t *testing.T, input func(string) string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	dir := t.TempDir()
	host, _, read := lifecycleObserver(t, dir)
	obj := input(dir)
	s := New(Options{NativeTLS: true, RegisterUnwind: true})
	defer s.Close()
	load(t, s, host, obj)
	if err := s.Link("missing_root"); err == nil {
		t.Fatal("missing root accepted")
	}
	expectEvents(t, read)
	call(t, s, "tls_lifecycle", 20, 22, 50)
	expectEvents(t, read, 7)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 7, 9, 10)
}

func TestNativeMachOTLSRetirement(t *testing.T) {
	needMachONativeTLS(t)
	testNativeTLSRetirement(t)
}

func testNativeTLSRetirement(t *testing.T, flags ...string) {
	host := nativeThreadLibrary(t, "testdata/emutls_threads.c")
	start, err := host.Resolve("tls_threads_pause")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var inputs []string
	for _, part := range []string{"consumer", "provider"} {
		inputs = append(inputs, compile(t, "testdata/emutls_"+part+".c", filepath.Join(dir, part+".o"), append([]string{"-O2"}, flags...)...))
	}
	for _, mode := range []string{"owner_before_join", "concurrent_exit"} {
		t.Run(mode, func(t *testing.T) {
			s := New(Options{NativeTLS: true})
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
			joined := false
			defer func() {
				defer lease.Close()
				if !joined {
					call(t, host, "tls_threads_release", 0, 0, 0)
				}
			}()
			address, err := lease.Address()
			if err != nil {
				t.Fatal(err)
			}
			if err := start.WithAddress(func(run uintptr) error { native.CallPointer(run, address); return nil }); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				if n, err := callInt32(host, "tls_threads_ready", 0, 0); err == nil && n == 4 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("native TLS workers did not reach retirement barrier")
				}
				time.Sleep(time.Millisecond)
			}
			owner := s.image.emulatedTLS
			if owner.LiveValues() != 4 {
				t.Fatal("missing native thread module storage")
			}
			closed := make(chan error, 1)
			go func() { closed <- s.Close() }()
			waitSessionRetired(t, s)
			select {
			case <-closed:
				t.Fatal("TLS image ignored retained code lease")
			default:
			}
			lease.Close()
			if mode == "concurrent_exit" {
				call(t, host, "tls_threads_release", 0, 0, 0)
				joined = true
			}
			if err := <-closed; err != nil || owner.LiveValues() != 0 {
				t.Fatal("TLS retirement failed:", err)
			}
			if !joined {
				call(t, host, "tls_threads_release", 0, 0, 0)
				joined = true
			}
		})
	}
}
