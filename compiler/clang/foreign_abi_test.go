package clang

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ramblelabs/dylib-go/abi"
)

func amd64CompilerContracts(t *testing.T) {
	t.Helper()
	path := headerFile(t, "int f(void *,int);")
	for _, target := range targets {
		for _, cc := range []abi.Convention{abi.Win64, abi.SysV64} {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"f"}})
			if err != nil {
				t.Fatal(err)
			}
			h.Functions[0].Signature.Convention = cc
			_, err = h.CgoSource("bindings", "Bindings")
			if (err == nil) != (target.arch == "amd64") {
				t.Fatalf("target %s cc %d: %v", target.triple, cc, err)
			}
		}
	}
	for _, target := range []string{"x86_64-unknown-linux-gnu", "x86_64-apple-macosx11", "x86_64-pc-windows-msvc", "x86_64-w64-windows-gnu"} {
		t.Run("amd64/"+target, func(t *testing.T) {
			names := []string{"ms_add", "ms_mixed", "ms_pair", "ms_large", "ms_factory", "ms_apply", "ms_forward", "sysv_add", "sysv_mixed", "sysv_pair", "sysv_large", "sysv_factory", "sysv_apply", "sysv_forward"}
			// Metadata probes use Clang's stdint definitions without a foreign libc sysroot.
			h, err := Parse(context.Background(), "../../testdata/foreign_abi.h", Options{Compiler: compilerTool(t), Target: target, Flags: []string{"-ffreestanding"}, Functions: names})
			if err != nil {
				t.Fatal(err)
			}
			foreignName, want := "ms", abi.Win64
			if h.Target.OS == "windows" {
				foreignName, want = "sysv", abi.SysV64
			}
			d, err := h.Lookup(foreignName + "_pair")
			if err != nil || d.Signature.Convention != want {
				t.Fatal("non-default ABI lost:", d, err)
			}
			factory, err := h.Lookup(foreignName + "_factory")
			if err != nil {
				t.Fatal(err)
			}
			if len(factory.FunctionPointers) != 1 || factory.FunctionPointers[0].Signature.Convention != want {
				t.Fatal("factory ABI lost:", factory)
			}
			source, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
				t.Fatal("unsupported direct llgo ABI accepted")
			}
			start := bytes.Index(source, []byte("/*\n")) + 3
			end := bytes.Index(source[start:], []byte("*/")) + start
			file := filepath.Join(t.TempDir(), "bridge.c")
			if err := os.WriteFile(file, source[start:end], 0600); err != nil {
				t.Fatal(err)
			}
			for _, file := range []string{file, "../../testdata/foreign_abi.c"} {
				output, err := exec.Command(compilerTool(t), "--target="+target, "-ffreestanding", "-femit-all-decls", "-S", "-emit-llvm", file, "-o", "-").CombinedOutput()
				marker := []byte("win64cc")
				if h.Target.OS == "windows" {
					marker = []byte("x86_64_sysvcc")
				}
				if err != nil || !bytes.Contains(output, marker) {
					t.Fatalf("missing native ABI %s: %v\n%s", marker, err, output)
				}
			}
		})
	}
	for _, cc := range []string{"ms_abi", "sysv_abi"} {
		for _, arch := range []string{"arm64", "386"} {
			if _, err := callingConvention(cc, Target{Arch: arch, OS: "windows"}); err == nil {
				t.Fatal("unqualified architecture accepted", cc, arch)
			}
		}
	}
}
