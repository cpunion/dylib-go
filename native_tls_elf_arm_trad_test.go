package dylib

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ramblelabs/dylib-go/internal/native"
)

func TestELFARMTraditionalTLSObject(t *testing.T) {
	path := compile(t, "testdata/elf_arm_tlsgd.S", filepath.Join(t.TempDir(), "gd.o"), "--target=aarch64-unknown-linux-gnu")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := parse(path, b)
	if err != nil || len(f.info.Unsupported) != 0 {
		t.Fatal(f, err)
	}
	objects := []*object{f.obj}
	if needed, err := checkELFTLS(objects, true); err != nil || !needed {
		t.Fatal(needed, err)
	}
	if _, err := checkELFTLS(objects, false); err == nil {
		t.Fatal("traditional TLS accepted without opt-in")
	}
	defs, err := definitions(objects)
	if err != nil {
		t.Fatal(err)
	}
	im := &image{objects: objects, defs: defs, pointerSize: 8, page: 4096}
	if _, err := im.layoutELFTLS(0); err != nil || im.elfTLS.descriptors || len(im.elfTLS.records) != 2 || im.elfTLS.align != 64 || im.elfTLS.size != 128 {
		t.Fatal(im.elfTLS, err)
	}
	pages, adds := 0, 0
	for _, r := range f.obj.relocs {
		if r.typ == 513 {
			pages++
		}
		if r.typ == 514 {
			adds++
		}
	}
	if pages != 2 || adds != 2 {
		t.Fatal("fixture lost initialized/zero-fill GD pairs:", pages, adds)
	}
}

func needARMTraditionalTLS(t *testing.T) {
	t.Helper()
	needELFNativeTLS(t)
	if runtime.GOARCH != "arm64" {
		t.Skip("traditional AArch64 TLS requires Linux arm64")
	}
}

func TestNativeELFARMTraditionalTLS(t *testing.T) {
	needARMTraditionalTLS(t)
	host := nativeThreadLibrary(t, "testdata/emutls_threads.c")
	runner, err := host.Resolve("tls_threads")
	if err != nil {
		t.Fatal(err)
	}
	for _, opt := range []string{"-O0", "-O2"} {
		for _, model := range []string{"global-dynamic", "local-dynamic"} {
			for _, mode := range []string{"trad_trad", "trad_desc", "desc_trad"} {
				t.Run(opt+"/"+model+"/"+mode, func(t *testing.T) {
					dir := t.TempDir()
					var inputs []string
					for n, part := range []string{"consumer", "provider"} {
						cc, flags := "gcc", []string{opt, "-mtls-dialect=trad", "-ftls-model=" + model}
						traditional := mode == "trad_trad" || mode == "trad_desc" && n == 0 || mode == "desc_trad" && n == 1
						if !traditional {
							cc, flags = compiler(), []string{opt, "-fno-emulated-tls"}
						}
						path := compileWith(t, cc, "testdata/emutls_"+part+".c", filepath.Join(dir, part+".o"), flags...)
						checkARMNativeTLSModel(t, path, traditional)
						inputs = append(inputs, path)
					}
					archive := filepath.Join(dir, "provider.a")
					poison := compile(t, "testdata/elf_tls_unowned.c", filepath.Join(dir, "poison.o"))
					command(t, "ar", "rcs", archive, poison, inputs[1])
					for _, kind := range []string{"object", "archive"} {
						t.Run(kind, func(t *testing.T) {
							runtime.LockOSThread()
							defer runtime.UnlockOSThread()
							s := New(Options{NativeTLS: true})
							defer s.Close()
							load(t, s, inputs[0])
							if err := s.Link("tls_step"); err == nil || s.image != nil {
								t.Fatal("missing TLS provider accepted:", err)
							}
							provider := inputs[1]
							if kind == "archive" {
								provider = archive
							}
							load(t, s, provider)
							for i := int32(0); i < 100; i++ {
								call(t, s, "tls_step", 20, 22, 49+i)
							}
							if n := s.image.emulatedTLS.LiveValues(); n != 1 {
								t.Fatal("models allocated separate modules:", n)
							}
							step, err := s.Resolve("tls_step")
							if err != nil {
								t.Fatal(err)
							}
							if err := runner.WithAddress(func(run uintptr) error {
								return step.WithAddress(func(fn uintptr) error { native.CallPointer(run, fn); return nil })
							}); err != nil {
								t.Fatal(err)
							}
							call(t, host, "tls_thread_result", 0, 0, 0)
							owner := s.image.emulatedTLS
							if owner.LiveValues() != 1 {
								t.Fatal("foreign thread TLS leaked")
							}
							if err := s.Close(); err != nil || owner.LiveValues() != 0 {
								t.Fatal("traditional TLS cleanup:", err)
							}
						})
					}
				})
			}
		}
	}
}

func checkARMNativeTLSModel(t *testing.T, path string, traditional bool) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := parse(path, b)
	if err != nil {
		t.Fatal(err)
	}
	gd, desc := false, false
	for _, r := range f.obj.relocs {
		gd = gd || r.typ == 513
		desc = desc || elfTLSDesc("arm64", r.typ)
	}
	if traditional && (!gd || desc) || !traditional && (!desc || gd) {
		t.Fatalf("producer did not emit the requested TLS model: GD=%t TLSDESC=%t traditional=%t", gd, desc, traditional)
	}
}

func TestNativeELFARMTraditionalTLSValues(t *testing.T) {
	needARMTraditionalTLS(t)
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			path := compileWith(t, "gcc", "testdata/macho_tls_values.c", filepath.Join(t.TempDir(), "values.o"), opt, "-mtls-dialect=trad")
			checkARMNativeTLSModel(t, path, true)
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			for round := 0; round < 3; round++ {
				s := New(Options{NativeTLS: true})
				defer s.Close()
				load(t, s, path)
				for i := int32(0); i < 20; i++ {
					call(t, s, "native_tls_values", 20, 22, 62+i)
					call(t, s, "native_tls_mixed", 20, 22, 42)
				}
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestNativeELFARMTraditionalTLSCXX(t *testing.T) {
	needARMTraditionalTLS(t)
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			dir := t.TempDir()
			path := compileWith(t, "gcc", "testdata/emutls_dtor_objects.cpp", filepath.Join(dir, "cxx.o"), opt, "-mtls-dialect=trad", "-fno-exceptions", "-fno-rtti", "-DDYLIB_TLS_DYNAMIC_INIT")
			checkARMNativeTLSModel(t, path, true)
			archive := filepath.Join(dir, "cxx.a")
			command(t, "ar", "rcs", archive, path)
			finalizer := compile(t, "testdata/emutls_dtor_finalizer.c", filepath.Join(dir, "finalizer.o"))
			for _, kind := range []string{"object", "archive"} {
				t.Run(kind, func(t *testing.T) {
					host := nativeThreadLibrary(t, "testdata/emutls_dtor_host.c")
					input := path
					if kind == "archive" {
						input = archive
					}
					s := threadDestructorSession(t, host, input, finalizer, true)
					step, err := s.Resolve("tls_step")
					if err != nil {
						t.Fatal(err)
					}
					start, err := host.Resolve("tls_threads_pause")
					if err != nil {
						t.Fatal(err)
					}
					lease, err := step.Acquire()
					if err != nil {
						t.Fatal(err)
					}
					defer lease.Close()
					entry, err := lease.Address()
					if err != nil {
						t.Fatal(err)
					}
					if err := start.WithAddress(func(run uintptr) error { native.CallPointer(run, entry); return nil }); err != nil {
						t.Fatal(err)
					}
					joined := false
					defer func() {
						if !joined {
							callInt32(host, "tls_threads_release", 0, 0)
						}
					}()
					waitThreadDestructorCount(t, host, "tls_threads_ready", 4)
					lease.Close()
					if err := s.Close(); !errors.Is(err, ErrPendingThreadDestructors) {
						t.Fatal("pending C++ destructors lost their image:", err)
					}
					call(t, host, "tls_threads_release", 0, 0, 0)
					joined = true
					call(t, host, "recorded_event", -1, 0, 24)
					call(t, host, "tls_dtor_sum", 0, 0, 14952)
					if err := s.Close(); err != nil {
						t.Fatal(err)
					}
					call(t, host, "recorded_event", -1, 0, 25)
					call(t, host, "tls_dtor_sum", 0, 0, 15852)
				})
			}
		})
	}
}

func TestNativeELFARMTraditionalTLSRetainedProvider(t *testing.T) {
	needARMTraditionalTLS(t)
	t.Setenv("CLANG", "gcc")
	testNativeTLSRetainedProvider(t, "-mtls-dialect=trad")
}

func TestNativeELFARMTraditionalTLSPinnedGoThreads(t *testing.T) {
	needARMTraditionalTLS(t)
	t.Setenv("CLANG", "gcc")
	testNativeTLSPinnedGoThreads(t, "-mtls-dialect=trad")
}

func TestNativeELFARMTraditionalTLSLifecycle(t *testing.T) {
	needARMTraditionalTLS(t)
	testNativeTLSLifecycleInput(t, func(dir string) string {
		return compileWith(t, "gcc", "testdata/emutls_lifecycle.c", filepath.Join(dir, "lifecycle.o"), "-mtls-dialect=trad")
	})
}

func TestNativeELFARMTraditionalTLSRetirement(t *testing.T) {
	needARMTraditionalTLS(t)
	t.Setenv("CLANG", "gcc")
	testNativeTLSRetirement(t, "-mtls-dialect=trad")
}
