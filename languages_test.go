package dylib

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/internal/native"
)

// These optional compiler probes exercise only the exported C ABI. CI selects
// an explicit comma-separated list, so a missing compiler cannot hide coverage.
// The legacy value 1 enables all probes applicable to this OS, with llgo enabled
// additionally by DYLIB_TEST_LLGO=1.
func selectedLanguages(t *testing.T) map[string]bool {
	t.Helper()
	value := os.Getenv("DYLIB_TEST_LANGUAGES")
	if value == "1" {
		value = "rust,zig,fortran,go"
		if runtime.GOOS == "darwin" {
			value += ",swift"
		}
		if os.Getenv("DYLIB_TEST_LLGO") == "1" {
			value += ",llgo"
		}
	}
	selected := make(map[string]bool)
	if value == "" || value == "0" {
		return selected
	}
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		switch name {
		case "rust", "zig", "fortran", "swift", "go", "llgo":
			selected[name] = true
		default:
			t.Fatalf("unknown DYLIB_TEST_LANGUAGES entry %q; use rust,zig,fortran,swift,go,llgo", name)
		}
	}
	return selected
}

func requireLanguageNative(t *testing.T) {
	t.Helper()
	if !native.Available() || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Fatal("selected language probes require a supported 64-bit host with native calls enabled")
	}
}

func languageTool(t *testing.T, variable, fallback string) string {
	t.Helper()
	tool := os.Getenv(variable)
	if tool == "" {
		tool = fallback
	}
	path, err := exec.LookPath(tool)
	if err != nil {
		t.Fatalf("selected language compiler %q is unavailable; install it or set %s: %v", tool, variable, err)
	}
	return path
}

func TestLanguageProducers(t *testing.T) {
	selected := selectedLanguages(t)
	if !selected["rust"] && !selected["zig"] && !selected["fortran"] {
		t.Skip("select rust,zig,fortran with DYLIB_TEST_LANGUAGES")
	}
	requireLanguageNative(t)
	for _, tc := range []struct {
		language, tool, variable string
		args                     func(string) []string
	}{
		{"rust", "rustc", "DYLIB_RUSTC", func(p string) []string {
			return []string{"--crate-type=lib", "--emit=obj", "-C", "panic=abort", "-C", "relocation-model=pic", "-O", "testdata/add.rs", "-o", p}
		}},
		{"zig", "zig", "DYLIB_ZIG", func(p string) []string {
			return []string{"build-obj", "testdata/add.zig", "-O", "ReleaseFast", "-fPIC", "-femit-bin=" + p}
		}},
		{"fortran", "gfortran", "DYLIB_FC", func(p string) []string {
			return []string{"-c", "-fPIC", "testdata/add.f90", "-o", p}
		}},
	} {
		if !selected[tc.language] {
			continue
		}
		t.Run(tc.language, func(t *testing.T) {
			tool := languageTool(t, tc.variable, tc.tool)
			p := filepath.Join(t.TempDir(), tc.language+".o")
			command(t, tool, tc.args(p)...)
			s := New(Options{})
			defer s.Close()
			load(t, s, p)
			call(t, s, tc.language+"_add", 20, 22, 42)
		})
	}
}

func TestSwiftSharedProducer(t *testing.T) {
	if !selectedLanguages(t)["swift"] {
		t.Skip("select swift with DYLIB_TEST_LANGUAGES")
	}
	if runtime.GOOS != "darwin" {
		t.Fatal("the Swift shared-library probe currently supports macOS only")
	}
	requireLanguageNative(t)
	compiler := languageTool(t, "DYLIB_SWIFTC", "swiftc")
	dir := t.TempDir()
	obj := filepath.Join(dir, "swift.o")
	command(t, compiler, "-parse-as-library", "-O", "-emit-object", "testdata/add.swift", "-o", obj)
	s := New(Options{})
	defer s.Close()
	if e := s.Load(obj); e == nil {
		t.Fatal("Swift runtime metadata must not be silently ignored")
	}
	lib := filepath.Join(dir, "swift.dylib")
	command(t, compiler, "-parse-as-library", "-O", "-emit-library", "testdata/add.swift", "-o", lib)
	load(t, s, lib)
	call(t, s, "swift_add", 20, 22, 42)
}

func TestGoSharedProducers(t *testing.T) {
	selected := selectedLanguages(t)
	if !selected["go"] && !selected["llgo"] {
		t.Skip("select go,llgo with DYLIB_TEST_LANGUAGES")
	}
	requireLanguageNative(t)
	for _, name := range []string{"go", "llgo"} {
		if !selected[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			compiler := languageTool(t, "DYLIB_"+strings.ToUpper(name), name)
			ext := ".so"
			if runtime.GOOS == "darwin" {
				ext = ".dylib"
			}
			if runtime.GOOS == "windows" {
				ext = ".dll"
			}
			path := filepath.Join(t.TempDir(), "plugin"+ext)
			command(t, compiler, "build", "-buildmode=c-shared", "-o", path, "./testdata/goplugin")
			// Go runtimes cannot be unloaded safely. A child process releases the
			// pinned library at exit, allowing Windows to remove its DLL afterward.
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(executable, "-test.run=^TestGoSharedProducerChild$", "-test.v")
			cmd.Env = append(os.Environ(), "DYLIB_TEST_SHARED_CHILD="+path)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("shared-library child: %v\n%s", err, output)
			}
		})
	}
}

func TestGoSharedProducerChild(t *testing.T) {
	path := os.Getenv("DYLIB_TEST_SHARED_CHILD")
	if path == "" {
		t.Skip("launched by TestGoSharedProducers after compiling a shared library")
	}
	requireLanguageNative(t)
	s := New(Options{KeepLibraries: true})
	defer s.Close()
	load(t, s, path)
	call(t, s, "go_add", 20, 22, 42)
}
