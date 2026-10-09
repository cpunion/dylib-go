//go:build libffi && cgo && (linux || darwin || windows)

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestREADMEBitcodeArchiveExample(t *testing.T) {
	llc := os.Getenv("DYLIB_LLC")
	if llc == "" {
		llc = "llc"
	}
	compiler, err := exec.LookPath(llc)
	if err != nil {
		if os.Getenv("DYLIB_TEST_REQUIRE_LLVM") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH]
	if arch == "" {
		t.Skip("unqualified architecture")
	}
	triple := arch + "-unknown-linux-gnu"
	switch runtime.GOOS {
	case "darwin":
		triple = arch + "-apple-macosx11"
	case "windows":
		triple = arch + "-pc-windows-msvc"
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "add.ll")
	ir := fmt.Sprintf("target triple = %q\ndefine i32 @add(i32 %%a,i32 %%b){\n %%result = add i32 %%a,%%b\n ret i32 %%result\n}\n", triple)
	if err := os.WriteFile(input, []byte(ir), 0600); err != nil {
		t.Fatal(err)
	}
	module, archive := filepath.Join(dir, "add.bc"), filepath.Join(dir, "library.a")
	commands := [][]string{{"llvm-as", "-o", module, input}, {"llvm-ar", "rcs", archive, module}}
	for _, command := range commands {
		if data, err := exec.Command(filepath.Join(filepath.Dir(compiler), command[0]), command[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", command[0], err, data)
		}
	}
	if got, err := call(archive); err != nil || got != abi.Int32(42) {
		t.Fatalf("README LLVM archive example: %+v, %v", got, err)
	}
}
