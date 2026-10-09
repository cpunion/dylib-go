package llvm

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const moduleLayout = "e-p:64:64-i64:64-i128:128-n8:16:32:64-S128"

func moduleFile(t *testing.T, dir, name, triple, layout, function string) string {
	t.Helper()
	var headers string
	if triple != "" {
		headers += fmt.Sprintf("target triple = %q\n", triple)
	}
	if layout != "" {
		headers += fmt.Sprintf("target datalayout = %q\n", layout)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(headers+"define i32 @"+function+"(i32 %a,i32 %b){%sum=add i32 %a,%b\nret i32 %sum\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func mergeTools(t *testing.T) MergeOptions {
	t.Helper()
	compiler := tool(t, "DYLIB_LLC", "llc")
	return MergeOptions{Options: Options{Compiler: compiler}, Linker: tool(t, "DYLIB_LLVM_LINK", filepath.Join(filepath.Dir(compiler), "llvm-link"))}
}

func assertNoMergeStorage(t *testing.T, dir string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "dylib-go-llvm-merge-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("merge storage leaked: %v, %v", files, err)
	}
}

func TestMergedModuleTargetsAndOwnership(t *testing.T) {
	opts := mergeTools(t)
	assembler := tool(t, "DYLIB_LLVM_AS", filepath.Join(filepath.Dir(opts.Compiler), "llvm-as"))
	for i, target := range objectTargets {
		t.Run(target.triple, func(t *testing.T) {
			dir := t.TempDir()
			layout := moduleLayout
			if target.arch == "386" {
				layout = "e-p:32:32-i64:64-i128:128-n8:16:32-S128"
			} else if target.arch == "arm64" {
				layout += "-Fn32"
			}
			first := moduleFile(t, dir, "-first with spaces.ll", target.triple, layout, "add")
			second := moduleFile(t, dir, "second.ll", target.triple, layout, "other")
			bitcode := filepath.Join(dir, "second.bc")
			if data, err := exec.Command(assembler, "-o", bitcode, second).CombinedOutput(); err != nil {
				t.Fatalf("assembler: %v\n%s", err, data)
			}
			data, err := os.ReadFile(bitcode)
			if err != nil {
				t.Fatal(err)
			}
			if i%2 != 0 && bytes.HasPrefix(data, []byte("BC\xc0\xde")) {
				wrapper := make([]byte, 20)
				binary.LittleEndian.PutUint32(wrapper, 0x0b17c0de)
				binary.LittleEndian.PutUint32(wrapper[8:], 20)
				binary.LittleEndian.PutUint32(wrapper[12:], uint32(len(data)))
				binary.LittleEndian.PutUint32(wrapper[16:], 0xffffffff)
				if err := os.WriteFile(bitcode, append(wrapper, data...), 0600); err != nil {
					t.Fatal(err)
				}
			}
			local := opts
			local.TempDir = dir
			cwd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if relative, err := filepath.Rel(cwd, dir); err == nil {
				local.TempDir = relative
			}
			object, err := CompileModules(context.Background(), []string{first, bitcode}, local)
			if err != nil {
				t.Fatal(err)
			}
			defer object.Close()
			if !filepath.IsAbs(object.Path) {
				t.Fatal("owned object path depends on working directory:", object.Path)
			}
			if object.Info.Format != target.format || object.Info.Arch != target.arch || object.Info.OS != target.os {
				t.Fatalf("merged target changed: %+v", object.Info)
			}
			if !reflect.DeepEqual(object.SourceDirectories, []string{dir}) {
				t.Fatal("source directories:", object.SourceDirectories)
			}
			if err := object.Close(); err != nil {
				t.Fatal(err)
			}
			if err := object.Close(); err != nil {
				t.Fatal("repeated close:", err)
			}
			assertNoMergeStorage(t, dir)
			for _, path := range []string{first, second, bitcode} {
				if _, err := os.Stat(path); err != nil {
					t.Fatal("Close removed caller input:", err)
				}
			}
		})
	}
}

func TestMergeRejectsTargetsAndLinkFailures(t *testing.T) {
	opts := mergeTools(t)
	dir := t.TempDir()
	opts.TempDir = dir
	first := moduleFile(t, dir, "first.ll", "x86_64-unknown-linux-gnu", moduleLayout, "add")
	for _, test := range []struct{ triple, layout, function, want string }{
		{"aarch64-unknown-linux-gnu", moduleLayout, "other", "mismatch"},
		{"x86_64-unknown-linux-gnu", "e-p:32:32-i64:64-n8:16:32:64-S128", "other", "mismatch"},
		{"", moduleLayout, "other", "explicit target"},
		{"x86_64-unknown-linux-gnu", "", "other", "explicit target"},
		{"x86_64-unknown-linux-gnu", moduleLayout, "add", "multiply defined"},
	} {
		second := moduleFile(t, dir, "second.ll", test.triple, test.layout, test.function)
		local := opts
		local.Compiler = filepath.Join(dir, "no compiler")
		if _, err := CompileModules(context.Background(), []string{first, second}, local); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("expected %q before compilation: %v", test.want, err)
		}
		assertNoMergeStorage(t, dir)
	}
	for _, data := range [][]byte{emptyCOFF(), []byte("invalid IR"), []byte("BC\xc0\xde invalid"), []byte("!<arch>\n"), []byte("!<thin>\n")} {
		path := filepath.Join(dir, "invalid module")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := CompileModules(context.Background(), []string{first, path}, opts); err == nil {
			t.Fatal("invalid module accepted")
		}
		assertNoMergeStorage(t, dir)
	}
	local := opts
	local.Compiler = filepath.Join(dir, "no compiler")
	if _, err := CompileModules(context.Background(), []string{first}, local); err == nil {
		t.Fatal("missing compiler accepted after merging")
	}
	assertNoMergeStorage(t, dir)
}

func TestModuleInputLimitsAndCancellation(t *testing.T) {
	dir := t.TempDir()
	input := moduleFile(t, dir, "input.ll", "x86_64-unknown-linux-gnu", moduleLayout, "add")
	opts := MergeOptions{Options: Options{TempDir: dir}, Linker: filepath.Join(dir, "no linker")}
	for _, paths := range [][]string{nil, make([]string, maxModules+1), {dir}, {filepath.Join(dir, "missing")}} {
		if _, err := CompileModules(context.Background(), paths, opts); err == nil {
			t.Fatal("invalid inputs accepted")
		}
		assertNoMergeStorage(t, dir)
	}
	if _, err := CompileModules(nil, []string{input}, opts); err == nil {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CompileModules(ctx, []string{input}, opts); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := CompileModules(context.Background(), []string{input}, opts); err == nil {
		t.Fatal("missing linker accepted")
	}
	assertNoMergeStorage(t, dir)
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the shared input budget with small real files, including duplicate
	// references: caller files are snapshotted separately for each explicit module.
	if _, _, err := stageModules(context.Background(), []string{input, input}, t.TempDir(), int64(len(data))); err == nil || !strings.Contains(err.Error(), "at most 0 bytes") {
		t.Fatal("combined snapshot limit:", err)
	}
	file, err := os.Create(filepath.Join(dir, "huge.ll"))
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(maxInputSize + 1)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileModules(context.Background(), []string{file.Name()}, opts); err == nil || !strings.Contains(err.Error(), "regular input") {
		t.Fatal("oversized snapshot:", err)
	}
	assertNoMergeStorage(t, dir)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	opts.Linker = executable
	t.Setenv("DYLIB_LLVM_COMPILER_HELPER", "wait")
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := CompileModules(ctx, []string{input}, opts); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("linker cancellation:", err)
	}
	assertNoMergeStorage(t, dir)
}

func TestCanonicalModuleTargets(t *testing.T) {
	data := []byte("; target triple = \"ignored\"\nsource_filename = \"target triple = \\22ignored\\22\"\ntarget datalayout = \"e-p:64:64\"\ntarget triple = \"x86_64-unknown-linux-gnu\"\n")
	want := moduleTarget{triple: `"x86_64-unknown-linux-gnu"`, layout: `"e-p:64:64"`}
	if got, err := canonicalModuleTarget(data); err != nil || got != want {
		t.Fatalf("canonical headers: %+v, %v", got, err)
	}
	for _, input := range [][]byte{nil, bytes.Replace(data, []byte(`"e-p:64:64"`), []byte(`""`), 1), append(append([]byte(nil), data...), []byte("target triple = \"duplicate\"\n")...)} {
		if _, err := canonicalModuleTarget(input); err == nil {
			t.Fatal("missing/empty/duplicate target header accepted")
		}
	}
}

func TestMergedModuleCompilerCancellation(t *testing.T) {
	opts := mergeTools(t)
	dir := t.TempDir()
	opts.TempDir = dir
	input := moduleFile(t, dir, "input.ll", "x86_64-unknown-linux-gnu", moduleLayout, "add")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	opts.Compiler = executable
	marker := filepath.Join(dir, "compiler-started")
	t.Setenv("DYLIB_LLVM_COMPILER_HELPER", "wait")
	t.Setenv("DYLIB_LLVM_WAIT_MARKER", marker)
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
	if _, err := CompileModules(ctx, []string{input}, opts); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation after entering the compiler:", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("compiler was never entered:", err)
	}
	assertNoMergeStorage(t, dir)
}
