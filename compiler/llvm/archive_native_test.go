//go:build cgo && (linux || darwin || windows)

package llvm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	examplecall "github.com/cpunion/dylib-go/examples/call"
)

func TestNativeBitcodeArchivesAndLazySelection(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" && runtime.GOARCH != "386" {
		t.Skip("no native call adapter for this architecture")
	}
	llc := tool(t, "DYLIB_LLC", "llc")
	clang := nativeClang(t)
	archiver := tool(t, "DYLIB_LLVM_AR", filepath.Join(filepath.Dir(llc), "llvm-ar"))
	dir := t.TempDir()
	produce := func(name, code string, bitcode bool) string {
		t.Helper()
		source := filepath.Join(dir, name+".c")
		if err := os.WriteFile(source, []byte(code), 0600); err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(dir, name+".o")
		flags := nativeCFlags(source, output)
		if bitcode {
			flags = append(flags, "-emit-llvm")
		}
		if data, err := exec.Command(clang, flags...).CombinedOutput(); err != nil {
			t.Fatalf("archive producer: %v\n%s", err, data)
		}
		return output
	}
	provider := produce("provider", "static int seed;\n__attribute__((constructor)) static void init(void){seed=10;}\nint add(int a,int b){return a+b+seed;}\n", true)
	bias := produce("bias", "int bias(int a,int b){return a+b-10;}\n", false)
	unused := produce("unused", "extern int missing_dependency(int,int);\n__attribute__((constructor)) static void broken(void){missing_dependency(0,0);}\nint unused(int a,int b){return missing_dependency(a,b);}\n", true)
	root := produce("caller", "extern int add(int,int);\nextern int bias(int,int);\nint entry(int a,int b){return add(a,b)+bias(0,0);}\n", false)
	// A filesystem extraction keyed by member name would lose one of these
	// distinct native/bitcode members and break dependency resolution.
	duplicate := func(source, child string) string {
		t.Helper()
		folder := filepath.Join(dir, child)
		if err := os.Mkdir(folder, 0700); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(folder, "same.o")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	bias, provider = duplicate(bias, "native"), duplicate(provider, "bitcode")
	for _, format := range []string{"gnu", "bsd"} {
		input := filepath.Join(dir, format+".a")
		if data, err := exec.Command(archiver, "--format="+format, "qcs", input, unused, bias, provider).CombinedOutput(); err != nil {
			t.Fatalf("llvm-ar: %v\n%s", err, data)
		}
		inputData, err := os.ReadFile(input)
		if err != nil {
			t.Fatal(err)
		}
		for _, rootObject := range []bool{false, true} {
			name := format + "-archive-root"
			if rootObject {
				name = format + "-object-dependencies"
			}
			t.Run(name, func(t *testing.T) {
				source := filepath.Join(t.TempDir(), "archive.a")
				if err := os.WriteFile(source, inputData, 0600); err != nil {
					t.Fatal(err)
				}
				archive, err := CompileArchive(context.Background(), source, Options{Compiler: llc})
				if err != nil {
					t.Fatal(err)
				}
				defer archive.Close()
				if len(archive.Info.Members) != 3 {
					t.Fatal("compiled archive lost a member")
				}
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
				if data, err := exec.Command(archiver, "t", archive.Path).CombinedOutput(); err != nil || strings.ReplaceAll(string(data), "\r\n", "\n") != "unused.o\nsame.o\nsame.o\n" {
					t.Fatalf("native archive names/order: %q, %v", data, err)
				}
				session := dylib.New(dylib.Options{})
				defer session.Close()
				if rootObject {
					if err := session.Load(root); err != nil {
						t.Fatal(err)
					}
				}
				if err := session.Load(archive.Path); err != nil {
					t.Fatal(err)
				}
				if err := archive.Close(); err != nil {
					t.Fatal(err)
				}
				symbol, want := "add", int32(52)
				if rootObject {
					symbol, want = "entry", 42
				}
				if err := session.Link(symbol); err != nil {
					t.Fatal("unused member must not become a dependency or run its initializer:", err)
				}
				fn, err := session.Resolve(symbol)
				if err != nil {
					t.Fatal(err)
				}
				if err := fn.WithAddress(func(address uintptr) error {
					got, err := examplecall.Int32(address, 20, 22)
					if err != nil || got != want {
						t.Fatalf("compiled archive %s: %d, %v; want %d", symbol, got, err, want)
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
				if _, err := session.Resolve("unused"); err == nil {
					t.Fatal("unused member was selected")
				}
			})
		}
	}
}
