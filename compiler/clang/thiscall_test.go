package clang

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
			if !bytes.Contains(source, []byte("abi.ThisCall")) {
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
			if runtime.GOOS == "windows" && runtime.GOARCH == "386" && abi.Available() {
				thiscallNativeCallers(t, h)
			}
		})
	}
	for _, target := range []Target{{OS: "linux", Arch: "386"}, {OS: "darwin", Arch: "amd64"}, {OS: "windows", Arch: "amd64"}, {OS: "windows", Arch: "arm64"}} {
		if _, err := callingConvention("thiscall", target); err == nil {
			t.Fatal("unqualified thiscall target accepted:", target)
		}
	}
}

// LLVM IR uses x86_thiscallcc for both producers, but their machine-level sret
// placement differs. Execute generated C with each caller/producer combination.
func thiscallNativeCallers(t *testing.T, h *Header) {
	t.Helper()
	d, err := h.Lookup("this_large")
	if err != nil {
		t.Fatal(err)
	}
	single := *h
	single.Functions = []Declaration{d}
	source, err := single.CgoSource("probe", "Probe")
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.Index(source, []byte("/*\n")) + 3
	end := bytes.Index(source[start:], []byte("*/")) + start
	code := string(source[start:end]) + fmt.Sprintf(`
extern void this_large(void);
int main(void) {
    int32_t state[8]={20,0};
    for (int i=0;i<100;i++) {
        __auto_type result=%s((uintptr_t)&this_large,state,22);
        if (result.f0!=20 || result.f1!=22 || result.f2!=20 || result.f3!=22 || state[1]!=i+1) return 1;
    }
    return 0;
}
`, fmt.Sprintf("dylib_go_%x_call0", []byte("Probe")))
	compiler := compilerTool(t)
	run := func(t *testing.T, args ...string) {
		t.Helper()
		if output, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
			t.Fatalf("thiscall native compiler: %v\n%s", err, output)
		}
	}
	for _, caller := range []string{"i686-pc-windows-msvc", "i686-w64-windows-gnu"} {
		t.Run("clang_caller/"+caller, func(t *testing.T) {
			for _, opt := range []string{"-O0", "-O2"} {
				t.Run(opt, func(t *testing.T) {
					dir := t.TempDir()
					path := filepath.Join(dir, "caller.c")
					if err := os.WriteFile(path, []byte(code), 0600); err != nil {
						t.Fatal(err)
					}
					producer, object, binary := filepath.Join(dir, "producer.o"), filepath.Join(dir, "caller.o"), filepath.Join(dir, "caller.exe")
					run(t, "--target="+h.Target.Triple, opt, "-ffreestanding", "-fno-stack-protector", "-c", "../../testdata/thiscall.c", "-o", producer)
					run(t, "--target="+caller, opt, "-ffreestanding", "-fno-stack-protector", "-c", path, "-o", object)
					args := []string{"--target=i686-w64-windows-gnu", object, producer, "-o", binary}
					if root := os.Getenv("DYLIB_NATIVE_SYSROOT"); root != "" {
						args = append(args, "--sysroot="+root, "--rtlib=libgcc")
					}
					run(t, args...)
					if output, err := exec.Command(binary).CombinedOutput(); err != nil {
						t.Fatalf("%s producer, %s caller %s: %v\n%s", h.Target.Triple, caller, opt, err, output)
					}
				})
			}
		})
	}
}
