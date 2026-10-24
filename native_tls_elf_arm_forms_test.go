package dylib

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ramblelabs/dylib-go/internal/native"
)

func TestELFARMTLSGDAddressForms(t *testing.T) {
	for form, name := range []string{"adr", "movw", "page"} {
		t.Run(name, func(t *testing.T) {
			path := compile(t, "testdata/elf_arm_tlsgd_forms.S", filepath.Join(t.TempDir(), "form.o"), "--target=aarch64-unknown-linux-gnu", fmt.Sprint("-DDYLIB_TLSGD_FORM=", form))
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(path, b)
			if err != nil || len(f.info.Unsupported) != 0 {
				t.Fatal(f, err)
			}
			want := [][]uint32{{512}, {515, 516}, {513, 514}}[form]
			for _, typ := range want {
				count := 0
				for _, r := range f.obj.relocs {
					if r.typ == typ {
						count++
					}
				}
				if count != 2 {
					t.Fatalf("expected global and local TLS references for relocation %d, got %d", typ, count)
				}
			}
		})
	}
}

func TestNativeELFARMTLSGDAddressForms(t *testing.T) {
	needARMTraditionalTLS(t)
	host := nativeThreadLibrary(t, "testdata/emutls_threads.c")
	runner, err := host.Resolve("tls_threads")
	if err != nil {
		t.Fatal(err)
	}
	for form, name := range []string{"adr", "movw", "page"} {
		for _, producer := range []string{"clang_desc", "gcc_trad"} {
			for _, opt := range []string{"-O0", "-O2"} {
				t.Run(name+"/"+producer+"/"+opt, func(t *testing.T) {
					dir := t.TempDir()
					consumer := compile(t, "testdata/elf_arm_tlsgd_forms.S", filepath.Join(dir, "form.o"), fmt.Sprint("-DDYLIB_TLSGD_FORM=", form))
					cc, flags := compiler(), []string{opt, "-fno-emulated-tls"}
					if producer == "gcc_trad" {
						cc, flags = "gcc", []string{opt, "-mtls-dialect=trad"}
					}
					provider := compileWith(t, cc, "testdata/emutls_provider.c", filepath.Join(dir, "provider.o"), flags...)
					checkARMNativeTLSModel(t, provider, producer == "gcc_trad")
					archive := filepath.Join(dir, "provider.a")
					command(t, "ar", "rcs", archive, provider)
					for _, kind := range []string{"object", "archive"} {
						t.Run(kind, func(t *testing.T) {
							runtime.LockOSThread()
							defer runtime.UnlockOSThread()
							s := New(Options{NativeTLS: true})
							defer s.Close()
							input := provider
							if kind == "archive" {
								input = archive
							}
							load(t, s, consumer, input)
							for i := int32(0); i < 100; i++ {
								call(t, s, "tls_form_step", 20, 22, 49+i)
							}
							step, err := s.Resolve("tls_form_step")
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
								t.Fatal("address forms split module storage or leaked foreign thread values")
							}
							if err := s.Close(); err != nil || owner.LiveValues() != 0 {
								t.Fatal("TLSGD address form cleanup:", err)
							}
						})
					}
				})
			}
		}
	}
}

func TestELFARMTLSGDAddressRanges(t *testing.T) {
	for _, offset := range []int64{-(1 << 20) - 1, -(1 << 20), -1, 0, 1, (1 << 20) - 1, 1 << 20} {
		t.Run(fmt.Sprint("adr/", offset), func(t *testing.T) {
			im, o := elfTLSValidationImage()
			o.info.Arch = "arm64"
			im.base = 1 << 24
			im.elfTLS = &elfTLSLayout{records: map[definition]uint64{{o, 1}: 0}}
			r := relocation{section: 1, symbol: 1, typ: 512}
			le.PutUint32(im.mem, 0x10000003) // adr x3, .
			place := im.base + 2<<20
			im.elfTLS.records[definition{o, 1}] = uint64((2 << 20) + offset)
			_, err := im.relocateELFTLS(o, r, im.mem[:4], place)
			valid := offset >= -(1<<20) && offset < 1<<20
			if !valid {
				if err == nil || le.Uint32(im.mem) != 0x10000003 {
					t.Fatal("ADR overflow accepted or changed instruction:", err)
				}
				return
			}
			ins := le.Uint32(im.mem)
			got := signExtend((ins>>29&3)|(ins>>5&0x7ffff)<<2, 21)
			if err != nil || got != offset || ins&31 != 3 {
				t.Fatal("ADR range/operand:", got, offset, err)
			}
		})
	}
	for _, offset := range []int64{-(1 << 32) - 1, -(1 << 32), -65537, -1, 0, 1, 65536, (1 << 32) - 1, 1 << 32} {
		t.Run(fmt.Sprint("movw/", offset), func(t *testing.T) {
			im, o := elfTLSValidationImage()
			o.info.Arch = "arm64"
			if uint64(^uintptr(0)) < 1<<34 {
				t.Skip("full signed 32-bit GOT deltas require a 64-bit test process")
			}
			base := uint64(1) << 34
			im.base = uintptr(base)
			if offset < 0 {
				im.gotStart = uint64(-offset)
			}
			off := uint64(0)
			if offset >= 0 {
				off = uint64(offset)
			}
			im.elfTLS = &elfTLSLayout{records: map[definition]uint64{{o, 1}: off}}
			// Start with the opposite sign opcode to check linker selection.
			start := uint32(0xd2a00003) // movz x3, #0, lsl #16
			if offset >= 0 {
				start = 0x92a00003 // movn x3, #0, lsl #16
			}
			le.PutUint32(im.mem, start)
			le.PutUint32(im.mem[4:], 0xf2800003)
			_, err := im.relocateELFTLS(o, relocation{section: 1, symbol: 1, typ: 515}, im.mem[:4], im.base)
			if offset < -(1<<32) || offset >= 1<<32 {
				if err == nil || le.Uint32(im.mem) != start {
					t.Fatal("MOVW overflow accepted or changed instruction:", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := im.relocateELFTLS(o, relocation{section: 1, offset: 4, symbol: 1, typ: 516}, im.mem[4:8], im.base+4); err != nil {
				t.Fatal(err)
			}
			hi, low := le.Uint32(im.mem), le.Uint32(im.mem[4:])
			value := uint64(hi>>5&0xffff) << 16
			if hi&(1<<30) == 0 {
				value = ^value
			}
			value = value&^0xffff | uint64(low>>5&0xffff)
			if int64(value) != offset || hi&31 != 3 || low&31 != 3 {
				t.Fatalf("MOVW pair = %d, want %d; %#x %#x", int64(value), offset, hi, low)
			}
		})
	}
}
