package clang

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cpunion/dylib-go/abi"
)

func init() {
	mode := os.Getenv("DYLIB_CLANG_HELPER")
	if mode == "" {
		return
	}
	for _, argument := range os.Args {
		if argument == "-dumpmachine" {
			fmt.Println(nativeTriple())
			os.Exit(0)
		}
	}
	switch mode {
	case "wait":
		if err := os.WriteFile(os.Getenv("DYLIB_CLANG_MARKER"), []byte("started"), 0600); err != nil {
			os.Exit(1)
		}
		time.Sleep(time.Minute)
	case "stdout", "stderr":
		output := os.Stdout
		if mode == "stderr" {
			output = os.Stderr
		}
		data := bytes.Repeat([]byte("x"), 32<<10)
		for i := 0; i < 16; i++ {
			if _, err := output.Write(data); err != nil {
				os.Exit(1)
			}
		}
		os.Exit(1)
	default:
		fmt.Println("invalid AST JSON")
	}
	os.Exit(0)
}

func TestInputsAndCompilerFailures(t *testing.T) {
	path := headerFile(t, "int add(int,int);\n")
	opts := Options{Compiler: filepath.Join(t.TempDir(), "missing compiler"), Functions: []string{"add"}}
	for _, names := range [][]string{nil, make([]string, maxFunctions+1), {"bad name"}, {"add", "add"}} {
		local := opts
		local.Functions = names
		if _, err := Parse(context.Background(), path, local); err == nil {
			t.Fatal("invalid requested names accepted")
		}
	}
	for _, path := range []string{filepath.Dir(path), path + ".missing"} {
		if _, err := Parse(context.Background(), path, opts); err == nil {
			t.Fatal("nonregular/missing header accepted")
		}
	}
	if _, err := Parse(nil, path, opts); err == nil {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Parse(ctx, path, opts); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := Parse(context.Background(), path, opts); err == nil {
		t.Fatal("missing compiler accepted")
	}
	huge := filepath.Join(t.TempDir(), "large.h")
	file, err := os.Create(huge)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(maxHeaderSize + 1)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(context.Background(), huge, opts); err == nil || !strings.Contains(err.Error(), "regular header") {
		t.Fatal("header size limit:", err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	opts.Compiler = executable
	t.Setenv("DYLIB_CLANG_HELPER", "invalid")
	if _, err := Parse(context.Background(), path, opts); err == nil || !strings.Contains(err.Error(), "invalid AST JSON") {
		t.Fatal("invalid compiler output:", err)
	}
	for _, input := range []string{"{}", `{"kind":"FunctionDecl"}`, `{"kind":"TranslationUnitDecl"}`} {
		if _, err := decodeDeclarations([]byte(input), nativeTriple(), []string{"add"}); err == nil {
			t.Fatal("missing AST/probe metadata accepted")
		}
	}
}

func TestOutputBudgetsAndActiveCancellation(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DYLIB_CLANG_HELPER", "stdout")
	if _, err := runCompiler(context.Background(), executable, nil, "", 128); err == nil || !strings.Contains(err.Error(), "exceeds 128 bytes") {
		t.Fatal("stdout budget:", err)
	}
	t.Setenv("DYLIB_CLANG_HELPER", "stderr")
	if _, err := runCompiler(context.Background(), executable, nil, "", 128); err == nil || !strings.Contains(err.Error(), "[diagnostics truncated]") || len(err.Error()) > maxDiagnostics+4096 {
		t.Fatal("diagnostic budget:", err)
	}
	t.Setenv("DYLIB_CLANG_HELPER", "wait")
	marker := filepath.Join(t.TempDir(), "compiler-started")
	t.Setenv("DYLIB_CLANG_MARKER", marker)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := os.Stat(marker); err == nil {
					cancel()
					return
				}
			}
		}
	}()
	path := headerFile(t, "int add(int,int);\n")
	if _, err := Parse(ctx, path, Options{Compiler: executable, Functions: []string{"add"}}); !errors.Is(err, context.Canceled) {
		t.Fatal("active compiler cancellation:", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("compiler was not entered:", err)
	}
}

func TestDeclarationSnapshotsAndGenerationValidation(t *testing.T) {
	header := &Header{Target: Target{Triple: "x86_64-unknown-linux-gnu", OS: "linux", Arch: "amd64", PointerSize: 8}, Functions: []Declaration{{Name: "add", Symbol: "add", Signature: abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}}}}
	declaration, err := header.Lookup("add")
	if err != nil {
		t.Fatal(err)
	}
	declaration.Signature.Args[0] = abi.F64
	if header.Functions[0].Signature.Args[0] != abi.I32 {
		t.Fatal("Lookup exposed mutable signature storage")
	}
	if _, err := declaration.WithTail(abi.I32); err == nil {
		t.Fatal("ordinary declaration accepted a variadic tail")
	}
	for _, name := range []string{"", "bad-name", "func", "_"} {
		if _, err := header.GoSource(name, "Declarations"); err == nil {
			t.Fatal("invalid Go package accepted:", name)
		}
		if _, err := header.GoSource("bindings", name); err == nil {
			t.Fatal("invalid Go variable accepted:", name)
		}
	}
	header.Target.Arch = "386"
	if _, err := header.GoSource("bindings", "Declarations"); err == nil {
		t.Fatal("inconsistent target metadata accepted")
	}
	var empty *Header
	if _, err := empty.Lookup("add"); err == nil {
		t.Fatal("nil header lookup accepted")
	}
	if _, err := empty.ForHost("add"); err == nil {
		t.Fatal("nil header host guard accepted")
	}
}
