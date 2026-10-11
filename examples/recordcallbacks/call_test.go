//go:build cgo && (linux || darwin || windows)

package recordcallbacks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/compiler/clang"
)

var names = []string{"callbacks_factory", "callbacks_subtract", "callbacks_echo", "callbacks_apply", "callbacks_pointer", "callbacks_slot", "callbacks_nested", "callbacks_nested_apply", "callbacks_store", "callbacks_run", "callbacks_clear"}

func tool(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	return path
}

func withFixtures(t *testing.T, use func(*testing.T, string)) {
	t.Helper()
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = "clang"
	}
	compiler, ar := tool(t, compiler), tool(t, "llvm-ar")
	compile := func(tool string, args ...string) {
		t.Helper()
		if data, err := exec.Command(tool, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", tool, err, data)
		}
	}
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			dir := t.TempDir()
			object, archive := filepath.Join(dir, "callbacks.o"), filepath.Join(dir, "callbacks.a")
			flags := []string{"--target=" + _BindingsDeclarations.Target.Triple, optimization, "-fno-stack-protector"}
			if runtime.GOOS != "windows" {
				flags = append(flags, "-fPIC")
			}
			compile(compiler, append(append([]string(nil), flags...), "-c", "testdata/exports.c", "-o", object)...)
			compile(ar, "rcs", archive, object)
			library, linkFlags := filepath.Join(dir, "callbacks.so"), []string{"-shared"}
			switch runtime.GOOS {
			case "darwin":
				library, linkFlags = filepath.Join(dir, "callbacks.dylib"), []string{"-dynamiclib"}
			case "windows":
				library, linkFlags = filepath.Join(dir, "callbacks.dll"), []string{"-shared", "-Wl,--export-all-symbols"}
			}
			if runtime.GOARCH == "386" {
				if runtime.GOOS == "linux" {
					linkFlags = append(linkFlags, "-m32")
				} else if runtime.GOOS == "windows" {
					linkFlags = append(linkFlags, "--target=i686-w64-windows-gnu")
					if root := os.Getenv("DYLIB_NATIVE_SYSROOT"); root != "" {
						linkFlags = append(linkFlags, "--sysroot="+root, "--rtlib=libgcc")
					}
				}
			}
			if runtime.GOOS == "linux" {
				linkFlags = append(linkFlags, "-fPIC")
			}
			compile(compiler, append(linkFlags, optimization, "testdata/exports.c", "-o", library)...)
			for _, input := range []struct{ name, path string }{{"object", object}, {"archive", archive}, {"library", library}} {
				t.Run(input.name, func(t *testing.T) { use(t, input.path) })
			}
		})
	}
}

func open(t *testing.T, path string) (*dylib.Session, *Bindings) {
	t.Helper()
	s, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	b, err := NewBindings(s)
	if err != nil {
		t.Fatal(err)
	}
	return s, b
}

func TestNativeCallbackRecords(t *testing.T) {
	withFixtures(t, func(t *testing.T, path string) {
		if got, err := Call(path); err != nil || got != 42 {
			t.Fatal(got, err)
		}
		s, b := open(t, path)
		// Retain this image while using entries returned in records or pointers.
		owner, err := s.Resolve("callbacks_factory")
		if err != nil {
			t.Fatal(err)
		}
		err = owner.WithAddress(func(uintptr) error {
			r, err := b.Callbacks_factory(1)
			if err != nil || r.Add == nil {
				t.Fatal(r, err)
			}
			r, err = b.Callbacks_echo(r)
			if err != nil {
				return err
			}
			if got, err := b.Callbacks_apply(r, 20, 21); err != nil || got != 42 {
				t.Fatal(got, err)
			}
			sub, err := b.Callbacks_subtract()
			if err != nil {
				return err
			}
			r.Add = sub.Add
			if got, err := b.Callbacks_apply(r, 63, 22); err != nil || got != 42 {
				t.Fatal(got, err)
			}
			nested, err := b.Callbacks_nested(BindingsRecord1{Items: [3]BindingsRecord0{r, sub, r}})
			if err != nil || nested.Items[2].Add != sub.Add {
				t.Fatal(nested, err)
			}
			if got, err := b.Callbacks_nested_apply(nested, 63, 22); err != nil || got != 42 {
				t.Fatal(got, err)
			}
			if err := b.Callbacks_store(r); err != nil {
				return err
			}
			if got, err := b.Callbacks_run(63, 22); err != nil || got != 42 {
				t.Fatal(got, err)
			}
			if err := b.Callbacks_clear(); err != nil {
				return err
			}
			if got, err := b.Callbacks_apply(BindingsRecord0{}, 20, 22); err != nil || got != -1 {
				t.Fatal(got, err)
			}
			return checkPointer(t, b)
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestGeneratedCallbackRecords(t *testing.T) {
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = "clang"
	}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: tool(t, compiler), Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := generate(h)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s_%s.go", backend, runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated callback-record bindings are stale")
	}
}
