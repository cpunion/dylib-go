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

func TestREADMECompilerExample(t *testing.T) {
	tool := os.Getenv("DYLIB_LLC")
	if tool == "" {
		tool = "llc"
	}
	if _, err := exec.LookPath(tool); err != nil {
		if os.Getenv("DYLIB_TEST_REQUIRE_LLVM") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH]
	triple := arch + "-unknown-linux-gnu"
	switch runtime.GOOS {
	case "darwin":
		triple = arch + "-apple-macosx11"
	case "windows":
		triple = arch + "-pc-windows-msvc"
	}
	input := filepath.Join(t.TempDir(), "add.ll")
	ir := fmt.Sprintf("target triple = %q\ndefine i32 @add(i32 %%a,i32 %%b){\n %%result = add i32 %%a,%%b\n ret i32 %%result\n}\n", triple)
	if err := os.WriteFile(input, []byte(ir), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := call(input); err != nil || got != abi.Int32(42) {
		t.Fatalf("README LLVM example: %+v, %v", got, err)
	}
}
