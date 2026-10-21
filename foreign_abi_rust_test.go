package dylib

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestNativeRustAMD64Conventions(t *testing.T) {
	if runtime.GOARCH != "amd64" || runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Rust alternate-ABI execution is qualified on Linux/macOS amd64")
	}
	if !selectedLanguages(t)["rust"] {
		t.Skip("select rust with DYLIB_TEST_LANGUAGES")
	}
	if !abi.Available() {
		t.Skip("dynamic Rust ABI calls require libffi")
	}
	requireLanguageNative(t)
	rustc := languageTool(t, "DYLIB_RUSTC", "rustc")
	for _, opt := range []string{"0", "2"} {
		t.Run("O"+opt, func(t *testing.T) {
			dir := t.TempDir()
			object := filepath.Join(dir, "rust.o")
			command(t, rustc, "--edition=2024", "--crate-type=lib", "--emit=obj", "-C", "panic=abort", "-C", "relocation-model=pic", "-C", "debug-assertions=no", "-C", "overflow-checks=no", "-C", "opt-level="+opt, "testdata/foreign_abi.rs", "-o", object)
			archive := filepath.Join(dir, "rust.a")
			command(t, "ar", "rcs", archive, object)
			library, flag := filepath.Join(dir, "rust.so"), "-shared"
			if runtime.GOOS == "darwin" {
				library, flag = filepath.Join(dir, "rust.dylib"), "-dynamiclib"
			}
			command(t, compiler(), flag, object, "-o", library)
			for _, input := range []struct{ kind, path string }{{"object", object}, {"archive", archive}, {"library", library}} {
				t.Run(input.kind, func(t *testing.T) {
					s := New(Options{ProcessSymbols: true})
					defer s.Close()
					load(t, s, input.path)
					for _, cc := range []struct {
						prefix string
						abi    abi.Convention
					}{{"ms", abi.Win64}, {"sysv", abi.SysV64}} {
						t.Run(cc.prefix, func(t *testing.T) { exerciseAMD64Convention(t, s, cc.prefix, cc.abi) })
					}
				})
			}
		})
	}
}
