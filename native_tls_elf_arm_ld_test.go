package dylib

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ramblelabs/dylib-go/internal/native"
)

func armTLSLDObject(t *testing.T, form int, flags ...string) string {
	t.Helper()
	return compile(t, "testdata/elf_arm_tlsld.S", filepath.Join(t.TempDir(), "ld.o"), append(flags, fmt.Sprint("-DDYLIB_TLSLD_FORM=", form))...)
}

func TestELFARMLocalDynamicTLSRecords(t *testing.T) {
	for form, name := range []string{"adr", "page", "movw", "literal"} {
		t.Run(name, func(t *testing.T) {
			path := armTLSLDObject(t, form, "--target=aarch64-unknown-linux-gnu")
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(path, b)
			if err != nil || len(f.info.Unsupported) != 0 {
				t.Fatal(f, err)
			}
			// Supply the external TLS definition without needing a target C sysroot.
			o := f.obj
			for i := range o.symbols {
				if o.symbols[i].name == "tls_shared" {
					if !o.symbols[i].tls || o.symbols[i].section != 0 {
						t.Fatal("external TLS declaration lost")
					}
					for n, sec := range o.sections {
						if sec != nil && sec.nativeTLS == elfTLSData {
							o.symbols[i].section, o.symbols[i].size = n, 4
							break
						}
					}
				}
			}
			if needed, err := checkELFTLS([]*object{o}, true); err != nil || !needed {
				t.Fatal(needed, err)
			}
			if _, err := checkELFTLS([]*object{o}, false); err == nil {
				t.Fatal("LD object accepted without NativeTLS")
			}
			defs, err := definitions([]*object{o})
			if err != nil {
				t.Fatal(err)
			}
			im := &image{objects: []*object{o}, defs: defs, pointerSize: 8, page: 4096}
			if _, err := im.layoutELFTLS(0); err != nil || len(im.elfTLS.records) != 1 || im.elfTLS.records[definition{}] == 0 || im.elfTLS.size <= 65536 {
				t.Fatal("LD references must share a zero-offset module index:", im.elfTLS, err)
			}
			seen := map[uint32]bool{}
			for _, r := range o.relocs {
				seen[r.typ] = true
			}
			want := [][]uint32{{517}, {518, 519}, {520, 521}, {522}}[form]
			for typ := uint32(523); typ <= 538; typ++ {
				want = append(want, typ)
			}
			want = append(want, 572, 573)
			for _, typ := range want {
				if !seen[typ] {
					t.Fatal("missing local-dynamic fixture relocation:", typ)
				}
			}
		})
	}
}

func TestNativeELFARMLocalDynamicTLS(t *testing.T) {
	needARMTraditionalTLS(t)
	testNativeELFARMLocalDynamicTLS(t, false, false)
}

func TestNativeELFARMLocalDynamicTLSCXX(t *testing.T) {
	needARMTraditionalTLS(t)
	testNativeELFARMLocalDynamicTLS(t, true, false)
}

func TestNativeELFARMLocalDynamicTLSSystemControl(t *testing.T) {
	needARMTraditionalTLS(t)
	testNativeELFARMLocalDynamicTLS(t, false, true)
}

func testNativeELFARMLocalDynamicTLS(t *testing.T, cxx, systemControl bool) {
	host := nativeThreadLibrary(t, "testdata/emutls_threads.c")
	runner, err := host.Resolve("tls_threads")
	if err != nil {
		t.Fatal(err)
	}
	for form, name := range []string{"adr", "page", "movw", "literal"} {
		if (cxx || systemControl) && name != "page" {
			continue // Same offset instructions, with compiler-native C++ lifetime.
		}
		for _, producer := range []string{"clang_desc", "gcc_trad"} {
			for _, opt := range []string{"-O0", "-O2"} {
				t.Run(name+"/"+producer+"/"+opt, func(t *testing.T) {
					dir := t.TempDir()
					var control []string
					if systemControl {
						// GNU ld 2.40 does not implement the 128-bit DTPREL
						// records. Their control uses ADD + ordinary Q loads/stores.
						control = []string{"-DDYLIB_TLSLD_SYSTEM_CONTROL"}
					}
					consumer := armTLSLDObject(t, form, control...)
					cc, flags := compiler(), []string{opt, "-fno-emulated-tls"}
					if producer == "gcc_trad" {
						cc, flags = "gcc", []string{opt, "-mtls-dialect=trad"}
					}
					source := "testdata/elf_arm_tlsld_provider.c"
					kinds := []string{"object", "archive"}
					if systemControl {
						kinds = []string{"library"}
					}
					if cxx {
						source, kinds = "testdata/elf_arm_tlsld_provider.cpp", []string{"object", "archive"}
						flags = append(flags, "-fno-exceptions", "-fno-rtti", "-mno-outline-atomics")
					}
					provider := compileWith(t, cc, source, filepath.Join(dir, "provider.o"), flags...)
					checkARMNativeTLSModel(t, provider, producer == "gcc_trad")
					archive, library := filepath.Join(dir, "provider.a"), filepath.Join(dir, "control.so")
					command(t, "ar", "rcs", archive, provider)
					if systemControl {
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
								if err := s.Link("tls_ld_step"); err == nil || s.image != nil {
									t.Fatal("missing LD provider accepted:", err)
								}
								load(t, s, input)
							}
							for i := int32(0); i < 100; i++ {
								call(t, s, "tls_ld_step", 20, 22, 49+i)
							}
							step, err := s.Resolve("tls_ld_step")
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
								t.Fatal("LD/GD/TLSDESC storage diverged or foreign-thread storage leaked")
							}
							if len(s.image.elfTLS.records) < 2 {
								t.Fatal("fixture lost mixed module-base and per-definition references")
							}
							if err := s.Close(); err != nil || owner.LiveValues() != 0 {
								t.Fatal("LD owner cleanup:", err)
							}
							// C++ global teardown traps unless all five TLS dtors ran.
						})
					}
				})
			}
		}
	}
}

func TestNativeELFARMLocalDynamicTLSRetirement(t *testing.T) {
	needARMTraditionalTLS(t)
	consumer := armTLSLDObject(t, 1, "-Dtls_ld_step=tls_step")
	provider := compile(t, "testdata/elf_arm_tlsld_provider.c", filepath.Join(t.TempDir(), "provider.o"), "-O2", "-fno-emulated-tls")
	testNativeTLSRetirementInputs(t, []string{consumer, provider})
}
