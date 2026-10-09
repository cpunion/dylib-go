//go:build cgo && (linux || darwin || windows)

package llvm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	examplecall "github.com/cpunion/dylib-go/examples/call"
)

func TestNativeIRAndBitcodeCalls(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" && runtime.GOARCH != "386" {
		t.Skip("no native call adapter for this architecture")
	}
	llc := tool(t, "DYLIB_LLC", "llc")
	clang := nativeClang(t)
	for _, kind := range []string{"ir", "bitcode"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "input.c")
			code := "static int seed;\n__attribute__((constructor)) static void init(void){seed=42;}\nint add(int a,int b){return a+b;}\nint initialized(int a,int b){return a+b+seed;}\n"
			if err := os.WriteFile(source, []byte(code), 0600); err != nil {
				t.Fatal(err)
			}
			input := filepath.Join(dir, "module")
			flags := append(nativeCFlags(source, input), "-emit-llvm")
			if kind == "ir" {
				flags = append(flags, "-S")
			}
			if data, err := exec.Command(clang, flags...).CombinedOutput(); err != nil {
				t.Fatalf("IR producer: %v\n%s", err, data)
			}
			object, err := Compile(context.Background(), input, Options{Compiler: llc})
			if err != nil {
				t.Fatal(err)
			}
			defer object.Close()
			session := dylib.New(dylib.Options{})
			defer session.Close()
			if err := session.Load(object.Path); err != nil {
				t.Fatal(err)
			}
			// Staging snapshots bytes; the compiler directory can be removed
			// before linking or executing the session-owned image.
			if err := object.Close(); err != nil {
				t.Fatal(err)
			}
			if err := session.Link(); err != nil {
				t.Fatal(err)
			}
			for _, entry := range []struct {
				name string
				want int32
			}{{"add", 42}, {"initialized", 84}} {
				fn, err := session.Resolve(entry.name)
				if err != nil {
					t.Fatal(err)
				}
				if err := fn.WithAddress(func(address uintptr) error {
					got, err := examplecall.Int32(address, 20, 22)
					if err != nil || got != entry.want {
						t.Fatalf("%s native %s: %d, %v", kind, entry.name, got, err)
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func nativeClang(t *testing.T) string {
	t.Helper()
	clangFallback := os.Getenv("CLANG")
	if clangFallback == "" {
		clangFallback = "clang"
	}
	return tool(t, "DYLIB_LLVM_CLANG", clangFallback)
}

func nativeCFlags(source, output string) []string {
	flags := []string{"-c", "-O0", "-fno-stack-protector", source, "-o", output}
	if runtime.GOOS != "windows" {
		flags = append(flags, "-fPIC")
	}
	if runtime.GOARCH == "386" {
		flags = append(flags, "-m32")
	}
	if runtime.GOOS == "windows" {
		arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH]
		flags = append(flags, "--target="+arch+"-pc-windows-msvc", "-ffreestanding")
	}
	return flags
}
