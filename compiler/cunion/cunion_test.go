package cunion_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/clang"
	"github.com/cpunion/dylib-go/compiler/cunion"
	example "github.com/cpunion/dylib-go/examples/cunion"
)

func functions() []cunion.Function {
	i := cunion.Value{Type: abi.I32, Union: "union Number", Member: "i"}
	result := append(example.Functions(), []cunion.Function{
		{Name: "store_number", Symbol: "union_store", Args: []cunion.Value{i}},
		{Name: "stored_number", Symbol: "union_stored", Result: cunion.Value{Type: abi.I32}},
		{Name: "read_number", Symbol: "union_read", Args: []cunion.Value{i}, Result: cunion.Value{Type: abi.I32}},
		{Name: "native_address", Symbol: "union_address", Result: cunion.Value{Type: abi.Pointer}},
		{Name: "read_pointer", Symbol: "union_pointer_read", Args: []cunion.Value{{Type: abi.Pointer, Union: "union Number", Member: "p"}}, Result: cunion.Value{Type: abi.I32}},
	}...)
	for _, entry := range []struct {
		name, member string
		kind         abi.Type
	}{
		{"i8", "i8", abi.I8}, {"u8", "u8", abi.U8}, {"i16", "i16", abi.I16}, {"u16", "u16", abi.U16}, {"i32", "i", abi.I32}, {"u32", "u", abi.U32}, {"i64", "i64", abi.I64}, {"u64", "u64", abi.U64}, {"f32", "f", abi.F32}, {"bool", "b", abi.Bool}, {"pointer", "p", abi.Pointer},
	} {
		v := cunion.Value{Type: entry.kind, Union: "a0", Member: entry.member}
		result = append(result, cunion.Function{Name: "target", Symbol: "union_" + entry.name, Args: []cunion.Value{v}, Result: v})
	}
	for _, entry := range []struct {
		name, typ, member string
		kind              abi.Type
	}{{"small", "union Small", "f", abi.F32}, {"large", "union Large", "i", abi.I32}, {"packed", "union Packed", "d", abi.F64}, {"aligned", "union Aligned", "i", abi.I32}} {
		v := cunion.Value{Type: entry.kind, Union: entry.typ, Member: entry.member}
		result = append(result, cunion.Function{Name: entry.name + "_add", Symbol: "union_" + entry.name, Args: []cunion.Value{v, {Type: entry.kind}}, Result: v})
	}
	return result
}
func tool(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	return p
}
func clangTool(t *testing.T) string {
	t.Helper()
	for _, key := range []string{"DYLIB_LLVM_CLANG", "CLANG"} {
		if v := os.Getenv(key); v != "" {
			return tool(t, v)
		}
	}
	return tool(t, "clang")
}
func write(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}
func render(t *testing.T) (*cunion.Facade, string, *clang.Header) {
	t.Helper()
	header, err := filepath.Abs("testdata/values.h")
	if err != nil {
		t.Fatal(err)
	}
	f, err := cunion.Generate(filepath.ToSlash(header), functions())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "facade.c")
	h := filepath.Join(dir, "facade.h")
	write(t, src, f.CSource)
	write(t, h, f.CHeader)
	m, err := clang.Parse(context.Background(), h, clang.Options{Compiler: clangTool(t), Flags: []string{"-ffreestanding"}, Functions: f.Functions})
	if err != nil {
		t.Fatal(err)
	}
	return f, src, m
}

func TestUnionFacadesAcrossTargets(t *testing.T) {
	f, src, _ := render(t)
	for _, target := range []string{"x86_64-unknown-linux-gnu", "aarch64-unknown-linux-gnu", "i686-unknown-linux-gnu", "x86_64-apple-macosx11", "arm64-apple-macosx11", "x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			args := []string{"--target=" + target, "-std=c17", "-ffreestanding", "-Werror=incompatible-pointer-types", "-c", src, "-o", filepath.Join(t.TempDir(), "facade.o")}
			if out, err := exec.Command(clangTool(t), args...).CombinedOutput(); err != nil {
				t.Fatalf("native union contract: %v\n%s", err, out)
			}
			h := filepath.Join(t.TempDir(), "facade.h")
			write(t, h, f.CHeader)
			if output, err := exec.Command(clangTool(t), "--target="+target, "-x", "c++", "-std=c++17", "-ffreestanding", "-fsyntax-only", h).CombinedOutput(); err != nil {
				t.Fatalf("C++ consumer header: %v\n%s", err, output)
			}
			m, err := clang.Parse(context.Background(), h, clang.Options{Compiler: clangTool(t), Target: target, Flags: []string{"-ffreestanding"}, Functions: f.Functions})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = m.CgoSource("binding", "Bindings"); err != nil {
				t.Fatal(err)
			}
			_, err = m.LLGoSource("binding", "Bindings")
			if (err != nil) != (m.Target.Arch == "386") {
				t.Fatal("llgo target qualification", err)
			}
		})
	}
}
func TestUnionManifestValidation(t *testing.T) {
	valid := functions()[0]
	tests := map[string]func(*cunion.Function){"native_name": func(f *cunion.Function) { f.Name = "x()" }, "export_name": func(f *cunion.Function) { f.Symbol = "a-b" }, "keyword": func(f *cunion.Function) { f.Symbol = "return" }, "member_name": func(f *cunion.Function) { f.Result.Member = "i;" }, "union_name": func(f *cunion.Function) { f.Result.Union = "struct Number" }, "member_without_union": func(f *cunion.Function) { f.Result.Union = "" }, "void_union": func(f *cunion.Function) { f.Result.Type = abi.Void }, "aggregate": func(f *cunion.Function) { f.Result.Type = abi.Struct }, "void_argument": func(f *cunion.Function) { f.Args = []cunion.Value{{}} }, "argument_limit": func(f *cunion.Function) { f.Args = make([]cunion.Value, 65) }, "native_collision": func(f *cunion.Function) { f.Symbol = f.Name }}
	for name, patch := range tests {
		t.Run(name, func(t *testing.T) {
			f := valid
			f.Args = append([]cunion.Value(nil), valid.Args...)
			patch(&f)
			if _, err := cunion.Generate("values.h", []cunion.Function{f}); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
	for _, header := range []string{"", "quote\".h", "back\\slash.h", "line\n.h", strings.Repeat("a", 4097)} {
		if _, err := cunion.Generate(header, []cunion.Function{valid}); err == nil {
			t.Fatal("invalid include accepted")
		}
	}
	for _, fs := range [][]cunion.Function{nil, {valid, valid}, make([]cunion.Function, 257)} {
		if _, err := cunion.Generate("values.h", fs); err == nil {
			t.Fatal("invalid exports accepted")
		}
	}
	a, err := example.Generate("values.h")
	if err != nil {
		t.Fatal(err)
	}
	b, err := example.Generate("values.h")
	if err != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("generation is not deterministic", err)
	}
}
func TestUnionCompilerRejectsInvalidContracts(t *testing.T) {
	for _, test := range []struct {
		name, kind, member, diagnostic string
		typ                            abi.Type
	}{
		{"not_union", "Fake", "i", "selected type must be a union", abi.I32},
		{"wrong_member", "union Number", "d", "incompatible union member transport", abi.I32},
		{"wrong_prototype", "union Other", "i", "incompatible native function prototype", abi.I32},
		{"bitfield", "union Bits", "i", "bit-field", abi.I32},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			header := filepath.Join(dir, "input.h")
			write(t, header, []byte("#include <stdint.h>\nunion Number {int32_t i; double d;}; union Other {int32_t i;}; typedef struct{int32_t i;} Fake; union Bits{signed int i:3;}; union Number target(union Number);\n"))
			v := cunion.Value{Type: test.typ, Union: test.kind, Member: test.member}
			f, err := cunion.Generate(filepath.ToSlash(header), []cunion.Function{{Name: "target", Symbol: "bridge", Args: []cunion.Value{v}, Result: v}})
			if err != nil {
				t.Fatal(err)
			}
			src := filepath.Join(dir, "facade.c")
			write(t, src, f.CSource)
			out, err := exec.Command(clangTool(t), "-std=c17", "-c", src, "-o", filepath.Join(dir, "facade.o")).CombinedOutput()
			if err == nil || !strings.Contains(string(out), test.diagnostic) {
				t.Fatalf("missing contract rejection: %v\n%s", err, out)
			}
		})
	}
}

func TestUnionSourceBudget(t *testing.T) {
	functions := make([]cunion.Function, 256)
	for i := range functions {
		functions[i] = cunion.Function{Name: "target", Symbol: "export_" + strconv.Itoa(i), Args: make([]cunion.Value, 64)}
		for j := range functions[i].Args {
			functions[i].Args[j] = cunion.Value{Type: abi.I32, Union: strings.Repeat("U", 256), Member: strings.Repeat("m", 256)}
		}
	}
	if _, err := cunion.Generate("values.h", functions); err == nil || !strings.Contains(err.Error(), "1 MiB") {
		t.Fatal("source budget", err)
	}
}
