package dylib

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Opt-in: these are independently compiled language producers, not required
// tools for users of this library. Each test exercises the C ABI subset only.
func TestLanguageProducers(t *testing.T) {
	if os.Getenv("DYLIB_TEST_LANGUAGES") != "1" {
		t.Skip("set DYLIB_TEST_LANGUAGES=1 for Rust/Zig/Fortran/Swift compiler probes")
	}
	needNative(t)
	for _, tc := range []struct {
		name, tool string
		args       func(string) []string
	}{
		{"rust_add", "rustc", func(p string) []string {
			return []string{"--crate-type=lib", "--emit=obj", "-C", "panic=abort", "-C", "relocation-model=pic", "-O", "testdata/add.rs", "-o", p}
		}},
		{"zig_add", "zig", func(p string) []string {
			return []string{"build-obj", "testdata/add.zig", "-O", "ReleaseFast", "-fPIC", "-femit-bin=" + p}
		}},
		{"fortran_add", "gfortran", func(p string) []string { return []string{"-c", "-fPIC", "testdata/add.f90", "-o", p} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), tc.name+".o")
			command(t, tc.tool, tc.args(p)...)
			s := New(Options{})
			defer s.Close()
			load(t, s, p)
			call(t, s, tc.name, 20, 22, 42)
		})
	}
}

func TestSwiftSharedProducer(t *testing.T) {
	if os.Getenv("DYLIB_TEST_LANGUAGES") != "1" || runtime.GOOS != "darwin" {
		t.Skip("opt-in macOS Swift probe")
	}
	needNative(t)
	dir := t.TempDir()
	obj := filepath.Join(dir, "swift.o")
	command(t, "swiftc", "-parse-as-library", "-O", "-emit-object", "testdata/add.swift", "-o", obj)
	s := New(Options{})
	defer s.Close()
	if e := s.Load(obj); e == nil {
		t.Fatal("Swift runtime metadata must not be silently ignored")
	}
	lib := filepath.Join(dir, "swift.dylib")
	command(t, "swiftc", "-parse-as-library", "-O", "-emit-library", "testdata/add.swift", "-o", lib)
	load(t, s, lib)
	call(t, s, "swift_add", 20, 22, 42)
}

func TestGoSharedProducers(t *testing.T) {
	if os.Getenv("DYLIB_TEST_LANGUAGES") != "1" {
		t.Skip("opt-in Go/llgo producer probe")
	}
	needNative(t)
	for _, compiler := range []string{"go", "llgo"} {
		t.Run(compiler, func(t *testing.T) {
			if compiler == "llgo" && os.Getenv("DYLIB_TEST_LLGO") != "1" {
				t.Skip("set DYLIB_TEST_LLGO=1")
			}
			ext := ".so"
			if runtime.GOOS == "darwin" {
				ext = ".dylib"
			}
			if runtime.GOOS == "windows" {
				ext = ".dll"
			}
			path := filepath.Join(t.TempDir(), "plugin"+ext)
			command(t, compiler, "build", "-buildmode=c-shared", "-o", path, "./testdata/goplugin")
			s := New(Options{KeepLibraries: true})
			defer s.Close()
			load(t, s, path)
			call(t, s, "go_add", 20, 22, 42)
		})
	}
}
