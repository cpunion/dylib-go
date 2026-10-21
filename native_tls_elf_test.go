package dylib

import (
	"bytes"
	"debug/elf"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestELFNativeTLSEntryEncoding(t *testing.T) {
	for _, tc := range []struct{ arch, target string }{{"amd64", "x86_64-unknown-linux-gnu"}, {"386", "i686-unknown-linux-gnu"}, {"arm64", "aarch64-unknown-linux-gnu"}} {
		t.Run(tc.arch, func(t *testing.T) {
			path := compile(t, "testdata/elf_tls_entry.S", filepath.Join(t.TempDir(), "entry.o"), "--target="+tc.target)
			f, err := elf.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			want, err := f.Section(".text").Data()
			if err != nil {
				t.Fatal(err)
			}
			got, err := elfTLSEntry(tc.arch, 0x12345678)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("entry differs from the independent assembler: %v\ngot %x\nwant%x", err, got, want)
			}
		})
	}
	if _, err := elfTLSEntry("amd64", 0); err == nil {
		t.Fatal("missing allocator accepted")
	}
	if _, err := elfTLSEntry("riscv64", 1); err == nil {
		t.Fatal("unsupported architecture accepted")
	}
}

func TestELFNativeTLSAcrossTargets(t *testing.T) {
	for _, target := range []string{"x86_64-unknown-linux-gnu", "i686-unknown-linux-gnu", "aarch64-unknown-linux-gnu"} {
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
			}
			if needed, err := checkELFTLS(objects, true); err != nil || !needed {
				t.Fatal(needed, err)
			}
			if _, err := checkELFTLS(objects, false); err == nil {
				t.Fatal("native TLS accepted without opt-in")
			}
			defs, err := definitions(objects)
			if err != nil {
				t.Fatal(err)
			}
			im := &image{objects: objects, defs: defs, pointerSize: uint64(objects[0].info.Bits / 8), page: 4096}
			if _, err := im.layoutELFTLS(0); err != nil || im.elfTLS.align != 64 || im.elfTLS.size < 68 || len(im.elfTLS.records) < 2 {
				t.Fatal(im.elfTLS, err)
			}
		})
	}
}

func needELFNativeTLS(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("native ELF TLS execution requires Linux")
	}
	needNative(t)
}

func TestNativeELFTLS(t *testing.T) { needELFNativeTLS(t); testNativeTLSModules(t) }
func TestNativeELFTLSRetainedProvider(t *testing.T) {
	needELFNativeTLS(t)
	testNativeTLSRetainedProvider(t)
}
func TestNativeELFTLSForeignThreads(t *testing.T) {
	needELFNativeTLS(t)
	testNativeTLSForeignThreads(t)
}
func TestNativeELFTLSValues(t *testing.T) { needELFNativeTLS(t); testNativeTLSValuesAndIsolation(t) }
func TestNativeELFTLSPinnedGoThreads(t *testing.T) {
	needELFNativeTLS(t)
	testNativeTLSPinnedGoThreads(t)
}
func TestNativeELFTLSLifecycle(t *testing.T)  { needELFNativeTLS(t); testNativeTLSLifecycle(t) }
func TestNativeELFTLSRetirement(t *testing.T) { needELFNativeTLS(t); testNativeTLSRetirement(t) }

func TestNativeELFTLSRegisters(t *testing.T) {
	needELFNativeTLS(t)
	if runtime.GOARCH != "arm64" {
		t.Skip("ELF TLSDESC preservation probe requires AArch64")
	}
	s := New(Options{NativeTLS: true})
	defer s.Close()
	dir := t.TempDir()
	load(t, s, compile(t, "testdata/elf_tls_registers.S", filepath.Join(dir, "registers.o")), compile(t, "testdata/emutls_provider.c", filepath.Join(dir, "provider.o"), "-fno-emulated-tls"))
	for i := 0; i < 100; i++ {
		call(t, s, "tls_registers", 20, 22, 42)
	}
}

func TestNativeELFTLSGNU(t *testing.T) {
	needELFNativeTLS(t)
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			dir := t.TempDir()
			s := New(Options{NativeTLS: true})
			defer s.Close()
			for _, part := range []string{"consumer", "provider"} {
				load(t, s, compileWith(t, "gcc", "testdata/emutls_"+part+".c", filepath.Join(dir, part+".o"), "-fPIC", opt))
			}
			for i := int32(0); i < 100; i++ {
				call(t, s, "tls_step", 20, 22, 49+i)
			}
		})
	}
}

func TestNativeELFTLSModels(t *testing.T) {
	needELFNativeTLS(t)
	for _, model := range []string{"global-dynamic", "local-dynamic", "initial-exec", "local-exec"} {
		t.Run(model, func(t *testing.T) {
			dir := t.TempDir()
			s := New(Options{NativeTLS: true})
			defer s.Close()
			for _, part := range []string{"consumer", "provider"} {
				load(t, s, compile(t, "testdata/emutls_"+part+".c", filepath.Join(dir, part+".o"), "-fno-emulated-tls", "-ftls-model="+model))
			}
			if model == "initial-exec" || model == "local-exec" {
				if err := s.Link("tls_step"); err == nil || !strings.Contains(err.Error(), "TLS") || s.image != nil {
					t.Fatal("static TLS model accepted:", err)
				}
			} else {
				runtime.LockOSThread()
				defer runtime.UnlockOSThread()
				for i := int32(0); i < 100; i++ {
					call(t, s, "tls_step", 20, 22, 49+i)
				}
			}
		})
	}
}
