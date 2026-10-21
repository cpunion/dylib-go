package dylib

import (
	"bytes"
	"debug/elf"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"

	"github.com/ramblelabs/dylib-go/internal/native"
)

func TestELFTLSDescEntryEncoding(t *testing.T) {
	for _, tc := range []struct{ arch, target string }{{"amd64", "x86_64-unknown-linux-gnu"}, {"386", "i686-unknown-linux-gnu"}} {
		t.Run(tc.arch, func(t *testing.T) {
			path := compile(t, "testdata/elf_tlsdesc_entry.S", filepath.Join(t.TempDir(), "entry.o"), "--target="+tc.target)
			f, err := elf.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			want, err := f.Section(".text").Data()
			if err != nil {
				t.Fatal(err)
			}
			got, err := elfTLSDescEntry(tc.arch, 0x12345678)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("entry differs from assembler: %v\ngot %x\nwant%x", err, got, want)
			}
		})
	}
	if _, err := elfTLSDescEntry("amd64", 0); err == nil {
		t.Fatal("missing allocator accepted")
	}
	if _, err := elfTLSDescEntry("arm64", 1); err == nil {
		t.Fatal("unsupported x86 architecture accepted")
	}
}

func TestELFTLSDescAcrossTargets(t *testing.T) {
	for _, target := range []string{"x86_64-unknown-linux-gnu", "i686-unknown-linux-gnu"} {
		t.Run(target, func(t *testing.T) {
			// Pure inspection CI may have Clang 17, which predates -mtls-dialect.
			// Its assembler can still produce the GNU2 records this parser consumes.
			path := compile(t, "testdata/elf_tlsdesc_relocations.S", filepath.Join(t.TempDir(), "relocs.o"), "--target="+target)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(path, data)
			if err != nil || len(f.info.Unsupported) != 0 {
				t.Fatal(f, err)
			}
			objects := []*object{f.obj}
			calls, addresses := 0, 0
			for _, r := range f.obj.relocs {
				if elfTLSDescCall(f.info.Arch, r.typ) {
					calls++
				} else if elfTLSDesc(f.info.Arch, r.typ) {
					addresses++
				}
			}
			if calls != 3 || addresses != 3 {
				t.Fatal("fixture lost global, local or module-base descriptor pairs:", calls, addresses)
			}
			if needed, err := checkELFTLS(objects, true); err != nil || !needed {
				t.Fatal(needed, err)
			}
			if _, err := checkELFTLS(objects, false); err == nil {
				t.Fatal("missing opt-in accepted")
			}
			defs, err := definitions(objects)
			if err != nil {
				t.Fatal(err)
			}
			im := &image{objects: objects, defs: defs, pointerSize: uint64(f.info.Bits / 8), page: 4096}
			if _, err := im.layoutELFTLS(0); err != nil || !im.elfTLS.descriptors || im.elfTLS.align != 64 || im.elfTLS.size < 68 || len(im.elfTLS.records) != 3 {
				t.Fatal(im.elfTLS, err)
			}
		})
	}
}

func needX86TLSDesc(t *testing.T) {
	t.Helper()
	needELFNativeTLS(t)
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "386" {
		t.Skip("x86 TLSDESC requires Linux amd64 or 386")
	}
}

// Both producers, optimization levels, archive extraction and mixed TLS models
// must agree on one per-image TLS allocation, on the caller and foreign threads.
func TestNativeELFTLSDesc(t *testing.T) {
	needX86TLSDesc(t)
	host := nativeThreadLibrary(t, "testdata/emutls_threads.c")
	runner, err := host.Resolve("tls_threads")
	if err != nil {
		t.Fatal(err)
	}
	for _, producer := range []string{"clang", "gcc"} {
		for _, opt := range []string{"-O0", "-O2"} {
			for _, mode := range []string{"desc_desc", "desc_gd", "gd_desc"} {
				t.Run(producer+"/"+opt+"/"+mode, func(t *testing.T) {
					dir := t.TempDir()
					cc := compiler()
					if producer == "gcc" {
						cc = "gcc"
					}
					inputs := make([]string, 2)
					for n, part := range []string{"consumer", "provider"} {
						dialect := "gnu2"
						if mode == "desc_gd" && n == 1 || mode == "gd_desc" && n == 0 {
							dialect = "gnu"
						}
						flags := []string{opt, "-mtls-dialect=" + dialect}
						if producer == "clang" {
							flags = append(flags, "-fno-emulated-tls")
						}
						inputs[n] = compileWith(t, cc, "testdata/emutls_"+part+".c", filepath.Join(dir, part+".o"), flags...)
					}
					archive := filepath.Join(dir, "provider.a")
					poison := compile(t, "testdata/elf_tls_unowned.c", filepath.Join(dir, "unowned.o"))
					command(t, "ar", "rcs", archive, poison, inputs[1])
					for _, kind := range []string{"object", "archive"} {
						t.Run(kind, func(t *testing.T) {
							runtime.LockOSThread()
							defer runtime.UnlockOSThread()
							s := New(Options{NativeTLS: true})
							defer s.Close()
							provider := inputs[1]
							if kind == "archive" {
								provider = archive
							}
							load(t, s, inputs[0], provider)
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
							if n := s.image.emulatedTLS.LiveValues(); n != 1 {
								t.Fatal("foreign thread TLS leaked:", n)
							}
							owner := s.image.emulatedTLS
							if err := s.Close(); err != nil || owner.LiveValues() != 0 {
								t.Fatal("descriptor cleanup:", err)
							}
						})
					}
				})
			}
		}
	}
}

func TestNativeELFTLSDescValues(t *testing.T) {
	needX86TLSDesc(t)
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			path := compile(t, "testdata/macho_tls_values.c", filepath.Join(t.TempDir(), "values.o"), opt, "-mtls-dialect=gnu2", "-fno-emulated-tls")
			for round := 0; round < 3; round++ {
				s := New(Options{NativeTLS: true, ProcessSymbols: true})
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

func TestNativeELFTLSDescCXX(t *testing.T) {
	needX86TLSDesc(t)
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			path := compile(t, "testdata/cli_tls.cpp", filepath.Join(t.TempDir(), "cxx.o"), opt, "-mtls-dialect=gnu2", "-fno-emulated-tls", "-fno-exceptions", "-fno-rtti")
			archive := filepath.Join(t.TempDir(), "cxx.a")
			command(t, "ar", "rcs", archive, path)
			for _, kind := range []string{"object", "archive"} {
				t.Run(kind, func(t *testing.T) {
					runtime.LockOSThread()
					defer runtime.UnlockOSThread()
					s := New(Options{NativeTLS: true})
					defer s.Close()
					input := path
					if kind == "archive" {
						input = archive
					}
					load(t, s, input)
					call(t, s, "cli_cxx_tls_add", 20, 22, 44)
					call(t, s, "cli_cxx_tls_add", 20, 22, 86)
					// The fixture's global finalizer traps unless its TLS destructor ran once.
					if err := s.Close(); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

func TestNativeELFTLSDescRegisters(t *testing.T) {
	needX86TLSDesc(t)
	for _, width := range []string{"128", "256", "512"} {
		t.Run(width, func(t *testing.T) {
			s := New(Options{})
			defer s.Close()
			load(t, s, compile(t, "testdata/elf_tlsdesc_probe.S", filepath.Join(t.TempDir(), "probe.o"), "-DDYLIB_TLS_PROBE_WIDTH="+width))
			supported, err := callInt32(s, "tlsdesc_probe_supported", 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if supported == 0 {
				t.Skip("CPU/OS does not enable the requested vector state")
			}
			probe, err := s.Resolve("tlsdesc_probe")
			if err != nil {
				t.Fatal(err)
			}
			helper, err := s.Resolve("tlsdesc_clobber")
			if err != nil {
				t.Fatal(err)
			}
			modes := []string{"automatic"}
			if width == "128" {
				modes = append(modes, "fxsave")
			}
			for _, mode := range modes {
				t.Run(mode, func(t *testing.T) {
					err := helper.WithAddress(func(address uintptr) error {
						code, err := elfTLSDescEntry(runtime.GOARCH, address)
						if err != nil {
							return err
						}
						if mode == "fxsave" {
							// Force only the feature-test immediate to zero, retaining every branch
							// and byte of the production fallback. No production hook is required.
							feature := []byte{0xf7, 0xc1, 0, 0, 0, 8}
							if bytes.Count(code, feature) != 1 {
								t.Fatal("missing unique OSXSAVE test")
							}
							code[bytes.Index(code, feature)+5] = 0
						}
						mem, err := native.Alloc(os.Getpagesize())
						if err != nil {
							return err
						}
						defer native.Free(mem)
						copy(mem, code)
						if err := native.Protect(mem, true, false); err != nil {
							return err
						}
						entry := uintptr(unsafe.Pointer(&mem[0]))
						for i := 0; i < 100; i++ {
							if err := probe.WithAddress(func(fn uintptr) error { native.CallPointer(fn, entry); return nil }); err != nil {
								return err
							}
							call(t, s, "tlsdesc_probe_result", 0, 0, 0)
						}
						return nil
					})
					if err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

func TestNativeELFTLSDescRetainedProvider(t *testing.T) {
	needX86TLSDesc(t)
	testNativeTLSRetainedProvider(t, "-fno-emulated-tls", "-mtls-dialect=gnu2")
}

func TestNativeELFTLSDescPinnedGoThreads(t *testing.T) {
	needX86TLSDesc(t)
	testNativeTLSPinnedGoThreads(t, "-fno-emulated-tls", "-mtls-dialect=gnu2")
}

func TestNativeELFTLSDescLifecycle(t *testing.T) {
	needX86TLSDesc(t)
	testNativeTLSLifecycle(t, "-fno-emulated-tls", "-mtls-dialect=gnu2")
}

func TestNativeELFTLSDescRetirement(t *testing.T) {
	needX86TLSDesc(t)
	testNativeTLSRetirement(t, "-fno-emulated-tls", "-mtls-dialect=gnu2")
}
