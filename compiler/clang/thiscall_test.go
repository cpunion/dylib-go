package clang

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

// Called by the ordinary convention test: qualify actual prototypes and emitted
// native calls, including hidden result pointers, on both Windows x86 ABIs.
func thiscallCompilerContracts(t *testing.T) {
	for _, target := range []string{"i686-pc-windows-msvc", "i686-w64-windows-gnu"} {
		t.Run("thiscall/"+target, func(t *testing.T) {
			names := []string{"this_add", "this_mixed", "this_wide", "this_pointer", "this_void", "this_payload", "this_large", "this_factory", "this_apply", "this_forward", "this_payload_apply"}
			h, err := Parse(context.Background(), "../../testdata/thiscall.h", Options{Compiler: compilerTool(t), Target: target, Functions: names})
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"this_add", "this_mixed", "this_wide", "this_pointer", "this_void", "this_payload", "this_large", "this_forward"} {
				d, err := h.Lookup(name)
				if err != nil {
					t.Fatal(err)
				}
				if d.Signature.Convention != abi.ThisCall || d.Symbol != name {
					t.Fatal("lost thiscall convention or symbol:", d)
				}
			}
			for _, name := range []string{"this_factory", "this_apply", "this_forward", "this_payload_apply"} {
				d, err := h.Lookup(name)
				if err != nil {
					t.Fatal(err)
				}
				if len(d.FunctionPointers) != 1 || d.FunctionPointers[0].Signature.Convention != abi.ThisCall {
					t.Fatal("lost native thiscall entry:", d)
				}
			}
			source, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(source, []byte("abi.ThisCall")) || !bytes.Contains(source, []byte("_result_pointer")) || !bytes.Contains(source, []byte(gccRegisterResult)) {
				t.Fatal("lost generated convention")
			}
			if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
				t.Fatal("unqualified direct llgo convention accepted")
			}
			start := bytes.Index(source, []byte("/*\n"))
			end := bytes.Index(source[start:], []byte("*/"))
			path := filepath.Join(t.TempDir(), "bridge.c")
			if err := os.WriteFile(path, source[start+3:start+end], 0600); err != nil {
				t.Fatal(err)
			}
			for _, file := range []string{path, "../../testdata/thiscall.c"} {
				out, err := exec.Command(compilerTool(t), "--target="+target, "-std=c17", "-ffreestanding", "-femit-all-decls", "-S", "-emit-llvm", file, "-o", "-").CombinedOutput()
				if err != nil || !bytes.Contains(out, []byte("x86_thiscallcc")) || !bytes.Contains(out, []byte("sret(")) {
					t.Fatalf("native thiscall lowering %s: %v\n%s", file, err, out)
				}
			}
		})
	}
	for _, target := range []Target{{OS: "linux", Arch: "386"}, {OS: "darwin", Arch: "amd64"}, {OS: "windows", Arch: "amd64"}, {OS: "windows", Arch: "arm64"}} {
		if _, err := callingConvention("thiscall", target); err == nil {
			t.Fatal("unqualified thiscall target accepted:", target)
		}
	}
}
