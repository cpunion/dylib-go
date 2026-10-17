package clang

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestVectorcallCgoAcrossWindowsTargets(t *testing.T) {
	path := headerFile(t, `typedef int (__attribute__((vectorcall)) *Entry)(int,int);
int __attribute__((vectorcall)) vector_add(int,int);
Entry factory(void); int apply(Entry,int,int);
int __vectorcall keyword(int,int);`)
	for _, target := range []string{"x86_64-pc-windows-msvc", "x86_64-w64-windows-gnu", "i686-pc-windows-msvc", "i686-w64-windows-gnu"} {
		t.Run(target, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target, Functions: []string{"vector_add", "factory", "apply", "keyword"}})
			if err != nil {
				t.Fatal(err)
			}
			if h.Functions[0].Signature.Convention != abi.VectorCall || h.Functions[0].Symbol != "vector_add@@8" || h.Functions[3].Signature.Convention != abi.VectorCall || h.Functions[3].Symbol != "keyword@@8" {
				t.Fatal("vectorcall metadata/decoration", h.Functions)
			}
			if h.Functions[1].FunctionPointers[0].Signature.Convention != abi.VectorCall || h.Functions[2].FunctionPointers[0].Signature.Convention != abi.VectorCall {
				t.Fatal("native entry convention lost", h.Functions)
			}
			source, err := h.CgoSource("binding", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(source, []byte("Convention: abi.VectorCall")) || !bytes.Contains(source, []byte("DYLIB_GO_VECTORCALL *")) {
				t.Fatal("vectorcall source metadata or native prototype missing")
			}
			if _, err := h.LLGoSource("binding", "Bindings"); err == nil {
				t.Fatal("unsupported direct llgo convention accepted")
			}
			start := bytes.Index(source, []byte("/*\n"))
			end := bytes.Index(source[start:], []byte("*/"))
			preamble := source[start+3 : start+end]
			cfile := filepath.Join(t.TempDir(), "bridge.c")
			if err := os.WriteFile(cfile, preamble, 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"--target=" + target, "-std=c17", "-ffreestanding", "-fsyntax-only", cfile}
			if out, err := exec.Command(compilerTool(t), args...).CombinedOutput(); err != nil {
				t.Fatalf("native vectorcall bridge: %v\n%s", err, out)
			}
			t.Run("unsupported_compiler", func(t *testing.T) {
				forced := append([]string{"-D__has_attribute(x)=0", "-Wno-builtin-macro-redefined"}, args...)
				out, err := exec.Command(compilerTool(t), forced...).CombinedOutput()
				if err == nil || !strings.Contains(string(out), "dylib_go_vectorcall_requires_a_supporting_C_compiler") {
					t.Fatalf("ignored calling convention accepted: %v\n%s", err, out)
				}
			})
		})
	}
}

func TestVectorcallRecordTargets(t *testing.T) {
	path := headerFile(t, `typedef struct{float a,b,c,d;} HFA; HFA __attribute__((vectorcall)) echo(HFA);`)
	for _, target := range []string{"x86_64-pc-windows-msvc", "x86_64-w64-windows-gnu", "i686-pc-windows-msvc", "i686-w64-windows-gnu"} {
		t.Run(target, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target, Functions: []string{"echo"}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = h.CgoSource("binding", "Bindings")
			unsupported := target == "i686-pc-windows-msvc"
			if (err != nil) != unsupported || unsupported && !strings.Contains(err.Error(), "MSVC i386 vectorcall record values") {
				t.Fatal("record target qualification", target, err)
			}
		})
	}
}

func TestVectorcallTargetAndSignatureValidation(t *testing.T) {
	for _, target := range []Target{{OS: "linux", Arch: "amd64"}, {OS: "darwin", Arch: "amd64"}, {OS: "windows", Arch: "arm64"}} {
		if _, err := callingConvention("vectorcall", target); err == nil {
			t.Fatal("unqualified convention accepted", target)
		}
	}
	s := abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32}, Convention: abi.VectorCall}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	s.Variadic, s.FixedArgs = true, 1
	if err := s.Validate(); err == nil {
		t.Fatal("vectorcall variadic signature accepted")
	}
}

// Compile every native fixture bridge on each qualified record ABI even when
// the current host cannot execute Windows code.
func TestVectorcallNativeFixtureBridges(t *testing.T) {
	names := []string{"vector_unsigned", "vector_bool", "vector_void", "vector_zero", "vector_add", "vector_wide", "vector_mixed", "vector_pair", "vector_hfa", "vector_large", "vector_pointer", "vector_factory", "vector_forward", "vector_apply", "vector_hfa_factory", "vector_hfa_forward"}
	for _, target := range []string{"x86_64-pc-windows-msvc", "x86_64-w64-windows-gnu", "i686-w64-windows-gnu"} {
		t.Run(target, func(t *testing.T) {
			h, err := Parse(context.Background(), "../../examples/vectorcall/testdata/exports.h", Options{Compiler: compilerTool(t), Target: target, Functions: names})
			if err != nil {
				t.Fatal(err)
			}
			source, err := h.CgoSource("main", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			start := bytes.Index(source, []byte("/*\n"))
			end := bytes.Index(source[start:], []byte("*/"))
			path := filepath.Join(t.TempDir(), "bridge.c")
			if err := os.WriteFile(path, source[start+3:start+end], 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(compilerTool(t), "--target="+target, "-ffreestanding", "-std=c17", "-femit-all-decls", "-S", "-emit-llvm", path, "-o", "-").CombinedOutput(); err != nil || !bytes.Contains(out, []byte("x86_vectorcallcc")) {
				t.Fatalf("missing native vectorcall lowering: %v\n%s", err, out)
			}
		})
	}
}
