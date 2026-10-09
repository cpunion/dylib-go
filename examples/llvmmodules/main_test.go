//go:build libffi && cgo && (linux || darwin || windows)

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestREADMECompiledModulesExample(t *testing.T) {
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = os.Getenv("CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	for _, tool := range []string{compiler, os.Getenv("DYLIB_LLC")} {
		if tool == "" {
			tool = "llc"
		}
		if _, err := exec.LookPath(tool); err != nil {
			if os.Getenv("DYLIB_TEST_REQUIRE_LLVM") == "1" {
				t.Fatal(err)
			}
			t.Skip(err)
		}
	}
	var inputs []string
	for i, code := range []string{
		"int provided(int a,int b){return a+b;}\n",
		"extern int provided(int,int);\nint add(int a,int b){return provided(a,b);}\n",
	} {
		dir := t.TempDir()
		source, output := filepath.Join(dir, "module.c"), filepath.Join(dir, "module")
		if err := os.WriteFile(source, []byte(code), 0600); err != nil {
			t.Fatal(err)
		}
		flags := []string{"-c", "-emit-llvm", "-fno-stack-protector", source, "-o", output}
		if i == 1 {
			flags = append(flags, "-S")
		}
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
		if data, err := exec.Command(compiler, flags...).CombinedOutput(); err != nil {
			t.Fatalf("module producer: %v\n%s", err, data)
		}
		inputs = append(inputs, output)
	}
	if got, err := call(inputs); err != nil || got != abi.Int32(42) {
		t.Fatalf("README merged module example: %+v, %v", got, err)
	}
}
