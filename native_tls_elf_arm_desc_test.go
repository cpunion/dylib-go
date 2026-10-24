package dylib

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ramblelabs/dylib-go/internal/native"
)

func armTLSDescObject(t *testing.T, form int, flags ...string) string {
	t.Helper()
	return compile(t, "testdata/elf_arm_tlsdesc_forms.S", filepath.Join(t.TempDir(), "desc.o"), append(flags, fmt.Sprint("-DDYLIB_TLSDESC_FORM=", form))...)
}

func TestELFARMTLSDescFormRecords(t *testing.T) {
	for form, name := range []string{"literal", "adr", "movw", "page"} {
		t.Run(name, func(t *testing.T) {
			path := armTLSDescObject(t, form, "--target=aarch64-unknown-linux-gnu")
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(path, b)
			if err != nil || len(f.info.Unsupported) != 0 {
				t.Fatal(f, err)
			}
			seen := map[uint32]int{}
			declared := false
			for _, sym := range f.obj.symbols {
				if sym.name == "tls_shared" {
					declared = sym.tls && sym.global && sym.section == 0
				}
			}
			if !declared {
				t.Fatal("missing external STT_TLS declaration")
			}
			for i := range f.obj.symbols {
				if f.obj.symbols[i].name == "tls_shared" {
					// An inspection-only definition, distinct from the local zero.
					for n, sec := range f.obj.sections {
						if sec != nil && sec.nativeTLS != 0 {
							f.obj.symbols[i].section, f.obj.symbols[i].value, f.obj.symbols[i].size = n, 4, 4
							break
						}
					}
				}
			}
			defs, err := definitions([]*object{f.obj})
			if err != nil {
				t.Fatal(err)
			}
			im := &image{objects: []*object{f.obj}, defs: defs, pointerSize: 8, page: 4096}
			if _, err := im.layoutELFTLS(0); err != nil || !im.elfTLS.descriptors || len(im.elfTLS.records) != 2 || im.elfTLS.align != 64 {
				t.Fatal("descriptor address records/layout:", im.elfTLS, err)
			}
			for _, r := range f.obj.relocs {
				seen[r.typ]++
			}
			want := [][]uint32{{560, 561, 569}, {561, 567, 568, 569}, {565, 566, 567, 568, 569}, {562, 563, 564, 569}}[form]
			for _, typ := range want {
				if seen[typ] != 2 || !elfTLSDesc("arm64", typ) {
					t.Fatal("global/local descriptor references lost:", typ, seen)
				}
			}
			if needed, err := checkELFTLS([]*object{f.obj}, true); err != nil || !needed {
				t.Fatal(needed, err)
			}
			if _, err := checkELFTLS([]*object{f.obj}, false); err == nil {
				t.Fatal("descriptor forms accepted without NativeTLS")
			}
		})
	}
}

func TestNativeELFARMTLSDescForms(t *testing.T) {
	needARMTraditionalTLS(t)
	testNativeELFARMTLSDescForms(t, false)
}

func TestNativeELFARMTLSDescFormsCXX(t *testing.T) {
	needARMTraditionalTLS(t)
	testNativeELFARMTLSDescForms(t, true)
}

func testNativeELFARMTLSDescForms(t *testing.T, cxx bool) {
	host := nativeThreadLibrary(t, "testdata/emutls_threads.c")
	runner, err := host.Resolve("tls_threads")
	if err != nil {
		t.Fatal(err)
	}
	for form, name := range []string{"literal", "adr", "movw", "page"} {
		for _, producer := range []string{"clang_desc", "gcc_trad"} {
			for _, opt := range []string{"-O0", "-O2"} {
				t.Run(name+"/"+producer+"/"+opt, func(t *testing.T) {
					dir := t.TempDir()
					consumer := armTLSDescObject(t, form)
					cc, flags := compiler(), []string{opt, "-fno-emulated-tls"}
					if producer == "gcc_trad" {
						cc, flags = "gcc", []string{opt, "-mtls-dialect=trad"}
					}
					source, kinds := "testdata/emutls_provider.c", []string{"object", "archive", "library"}
					if cxx {
						source, kinds = "testdata/elf_arm_tlsdesc_provider.cpp", []string{"object", "archive"}
						flags = append(flags, "-fno-exceptions", "-fno-rtti", "-mno-outline-atomics")
					}
					provider := compileWith(t, cc, source, filepath.Join(dir, "provider.o"), flags...)
					checkARMNativeTLSModel(t, provider, producer == "gcc_trad")
					archive, library := filepath.Join(dir, "provider.a"), filepath.Join(dir, "control.so")
					command(t, "ar", "rcs", archive, provider)
					if !cxx {
						// Use the identical objects with GNU ld and OS TLS too.
						command(t, "gcc", "-shared", consumer, provider, "-o", library)
					}
					for _, kind := range kinds {
						t.Run(kind, func(t *testing.T) {
							runtime.LockOSThread()
							defer runtime.UnlockOSThread()
							s := New(Options{NativeTLS: true})
							defer s.Close()
							input := provider
							if kind == "archive" {
								input = archive
							}
							if kind == "library" {
								load(t, s, library)
							} else {
								load(t, s, consumer)
								if err := s.Link("tls_desc_step"); err == nil || s.image != nil {
									t.Fatal("missing provider accepted:", err)
								}
								load(t, s, input)
							}
							for i := int32(0); i < 100; i++ {
								call(t, s, "tls_desc_step", 20, 22, 49+i)
							}
							step, err := s.Resolve("tls_desc_step")
							if err != nil {
								t.Fatal(err)
							}
							if err := runner.WithAddress(func(run uintptr) error {
								return step.WithAddress(func(fn uintptr) error { native.CallPointer(run, fn); return nil })
							}); err != nil {
								t.Fatal(err)
							}
							call(t, host, "tls_thread_result", 0, 0, 0)
							if kind == "library" {
								if err := s.Close(); err != nil {
									t.Fatal(err)
								}
								return
							}
							owner := s.image.emulatedTLS
							if owner.LiveValues() != 1 {
								t.Fatal("mixed descriptor/GD storage split or foreign thread storage leaked")
							}
							if err := s.Close(); err != nil || owner.LiveValues() != 0 {
								t.Fatal("descriptor owner cleanup:", err)
							}
							// C++ global teardown checks the receiver and five TLS dtors.
						})
					}
				})
			}
		}
	}
}

func TestNativeELFARMTLSDescFormRegisters(t *testing.T) {
	needARMTraditionalTLS(t)
	for form, name := range []string{"literal", "adr", "movw", "page"} {
		t.Run(name, func(t *testing.T) {
			s := New(Options{NativeTLS: true})
			defer s.Close()
			dir := t.TempDir()
			probe := compile(t, "testdata/elf_tls_registers.S", filepath.Join(dir, "registers.o"), fmt.Sprint("-DDYLIB_TLSDESC_FORM=", form))
			provider := compile(t, "testdata/emutls_provider.c", filepath.Join(dir, "provider.o"), "-fno-emulated-tls")
			load(t, s, probe, provider)
			for i := 0; i < 100; i++ {
				call(t, s, "tls_registers", 20, 22, 42)
			}
		})
	}
}

func TestNativeELFARMTLSDescFormRetirement(t *testing.T) {
	needARMTraditionalTLS(t)
	consumer := armTLSDescObject(t, 2, "-Dtls_desc_step=tls_step")
	provider := compile(t, "testdata/emutls_provider.c", filepath.Join(t.TempDir(), "provider.o"), "-O2", "-fno-emulated-tls")
	testNativeTLSRetirementInputs(t, []string{consumer, provider})
}
