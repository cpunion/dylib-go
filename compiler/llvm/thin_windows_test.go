//go:build cgo && windows

package llvm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	examplecall "github.com/cpunion/dylib-go/examples/call"
	"github.com/cpunion/dylib-go/internal/ar"
)

func TestNativeCompiledThinDLLDependencies(t *testing.T) {
	llc := tool(t, "DYLIB_LLC", "llc")
	clang := nativeClang(t)
	linker := tool(t, "DYLIB_LLVM_LLD", "lld-link")
	dir := t.TempDir()
	source := filepath.Join(dir, "provider.c")
	if err := os.WriteFile(source, []byte("int add(int a,int b){return a+b;}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	object := filepath.Join(dir, "provider.obj")
	if data, err := exec.Command(clang, nativeCFlags(source, object)...).CombinedOutput(); err != nil {
		t.Fatalf("DLL object: %v\n%s", err, data)
	}
	definition := filepath.Join(dir, "provider.def")
	if err := os.WriteFile(definition, []byte("LIBRARY provider.dll\nEXPORTS\nadd @7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dll, imports := filepath.Join(dir, "provider.dll"), filepath.Join(dir, "provider.lib")
	machine := map[string]string{"amd64": "X64", "arm64": "ARM64", "386": "X86"}[runtime.GOARCH]
	if data, err := exec.Command(linker, "/dll", "/noentry", "/nodefaultlib", "/machine:"+machine, "/def:"+definition, "/out:"+dll, "/implib:"+imports, object).CombinedOutput(); err != nil {
		t.Fatalf("DLL linker: %v\n%s", err, data)
	}
	data, err := os.ReadFile(imports)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ar.Decode(data, false, maxInputSize, maxArchiveMembers)
	if err != nil {
		t.Fatal(err)
	}
	var direct, proxy []thinReference
	for i, entry := range entries {
		path := filepath.Join(dir, fmt.Sprintf("import-%d.obj", i))
		if err := os.WriteFile(path, entry.Data, 0600); err != nil {
			t.Fatal(err)
		}
		direct = append(direct, thinReference{name: path})
		proxy = append(proxy, thinReference{name: imports, offset: entry.Offset})
	}
	for i, references := range [][]thinReference{direct, proxy} {
		input := filepath.Join(t.TempDir(), "imports.a")
		thinArchive(t, input, references...)
		archive, err := CompileArchive(context.Background(), input, Options{Compiler: llc})
		if err != nil {
			t.Fatal(err)
		}
		defer archive.Close()
		if len(archive.SourceDirectories) != 2 || archive.SourceDirectories[0] != dir {
			t.Fatal("missing DLL dependency origin:", archive.SourceDirectories)
		}
		session := dylib.New(dylib.Options{LibraryPaths: archive.SourceDirectories})
		if err := session.Load(archive.Path); err != nil {
			session.Close()
			t.Fatal(err)
		}
		if err := archive.Close(); err != nil {
			session.Close()
			t.Fatal(err)
		}
		if err := session.Link("add"); err != nil {
			session.Close()
			t.Fatal("DLL beside original thin/proxy members was not found:", err)
		}
		fn, err := session.Resolve("add")
		if err != nil {
			session.Close()
			t.Fatal(err)
		}
		err = fn.WithAddress(func(address uintptr) error {
			got, err := examplecall.Int32(address, 20, 22)
			if err != nil || got != 42 {
				t.Errorf("thin DLL case %d: %d, %v", i, got, err)
			}
			return err
		})
		cleanup := session.Close()
		if err != nil || cleanup != nil {
			t.Fatal(err, cleanup)
		}
	}
}
