package clang

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func compilerTool(t *testing.T) string {
	t.Helper()
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = os.Getenv("CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	path, err := exec.LookPath(compiler)
	if err != nil {
		if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	return path
}

func headerFile(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "header with spaces.h")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

var targets = []struct{ triple, os, arch string }{
	{"x86_64-unknown-linux-gnu", "linux", "amd64"},
	{"aarch64-unknown-linux-gnu", "linux", "arm64"},
	{"i686-unknown-linux-gnu", "linux", "386"},
	{"x86_64-apple-macosx11", "darwin", "amd64"},
	{"arm64-apple-macosx11", "darwin", "arm64"},
	{"x86_64-pc-windows-msvc", "windows", "amd64"},
	{"aarch64-pc-windows-msvc", "windows", "arm64"},
	{"i686-pc-windows-msvc", "windows", "386"},
}

func TestNativeAndForeignPrimitiveDeclarations(t *testing.T) {
	compiler := compilerTool(t)
	path := headerFile(t, "typedef unsigned long size_type;\nint add(int,int);\nlong wide(long,size_type);\n_Bool truth(_Bool);\ndouble mixed(float,double);\nconst void *pointer(const void *);\nchar character(char);\nint variable(int,...);\nvoid zero(void);\n")
	names := []string{"add", "wide", "truth", "mixed", "pointer", "character", "variable", "zero"}
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			header, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: names})
			if err != nil {
				t.Fatal(err)
			}
			if header.Target.OS != target.os || header.Target.Arch != target.arch || len(header.Functions) != len(names) {
				t.Fatalf("target/declarations: %+v", header)
			}
			wide, unsigned := abi.I64, abi.U64
			if target.os == "windows" || target.arch == "386" {
				wide, unsigned = abi.I32, abi.U32
			}
			want := []abi.Signature{
				{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}},
				{Result: wide, Args: []abi.Type{wide, unsigned}},
				{Result: abi.Bool, Args: []abi.Type{abi.Bool}},
				{Result: abi.F64, Args: []abi.Type{abi.F32, abi.F64}},
				{Result: abi.Pointer, Args: []abi.Type{abi.Pointer}},
				{Result: abi.I8, Args: []abi.Type{abi.I8}},
				{Result: abi.I32, Args: []abi.Type{abi.I32}, Variadic: true, FixedArgs: 1},
				{Result: abi.Void},
			}
			if target.os == "linux" && target.arch == "arm64" {
				want[5].Result, want[5].Args[0] = abi.U8, abi.U8
			}
			for i := range want {
				want[i].Convention = abi.CDecl
				if !reflect.DeepEqual(header.Functions[i].Signature, want[i]) || header.Functions[i].Symbol != names[i] {
					t.Fatalf("%s: %+v, want %+v", names[i], header.Functions[i], want[i])
				}
			}
			_, err = header.ForHost("add")
			if (err == nil) != (target.os == runtime.GOOS && target.arch == runtime.GOARCH) {
				t.Fatal("foreign target host guard:", err)
			}
			function, err := header.Lookup("variable")
			if err != nil {
				t.Fatal(err)
			}
			function, err = function.WithTail(abi.F32, abi.I8)
			if err != nil || len(function.Signature.Args) != 3 || len(header.Functions[6].Signature.Args) != 1 {
				t.Fatal("variadic prefix expansion/snapshot:", err)
			}
			source, err := header.GoSource("bindings", "Declarations")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
				t.Fatal("generated Go syntax:", err)
			}
		})
	}
}

func TestCallingConventionsAndUnsupportedDeclarations(t *testing.T) {
	compiler := compilerTool(t)
	for _, test := range []struct{ source, want string }{
		{"int f();", "explicit prototype"},
		{"static int f(int);", "static/inline"},
		{"inline int f(int);", "static/inline"},
		{"long double f(long double);", "unsupported C type"},
		{"union Pair{int a,b;}; union Pair f(union Pair);", "unsupported C type"},
		{"enum Kind{A,B}; enum Kind f(enum Kind);", "unsupported C type"},
		{"int __attribute__((vectorcall)) f(int);", "unsupported function"},
	} {
		path := headerFile(t, test.source)
		if _, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: "i686-pc-windows-msvc", Functions: []string{"f"}}); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("unsupported declaration %q: %v", test.source, err)
		}
	}
	path := headerFile(t, "int __attribute__((stdcall)) std_add(int,int);\nint __attribute__((fastcall)) fast_add(int,int);\n")
	header, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: "i686-pc-windows-msvc", Functions: []string{"std_add", "fast_add"}})
	if err != nil {
		t.Fatal(err)
	}
	if header.Functions[0].Signature.Convention != abi.StdCall || header.Functions[0].Symbol != "std_add@8" || header.Functions[1].Signature.Convention != abi.FastCall || header.Functions[1].Symbol != "@fast_add@8" {
		t.Fatal("calling conventions/symbol decoration:", header.Functions)
	}
}

func TestIncludesPreprocessingAndDeclarationSelection(t *testing.T) {
	compiler := compilerTool(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "api.h")
	if err := os.WriteFile(filepath.Join(dir, "types.h"), []byte("typedef unsigned short Code;\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#include \"types.h\"\n#ifdef API_ENABLED\nCode selected(Code);\n#endif\nint selected(Code);\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Incompatible redeclarations are a compiler error, rather than a last-wins
	// signature choice. Includes are resolved beside the original header.
	opts := Options{Compiler: compiler, Flags: []string{"-DAPI_ENABLED"}, Functions: []string{"selected"}}
	if _, err := Parse(context.Background(), path, opts); err == nil {
		t.Fatal("incompatible redeclarations accepted")
	}
	if err := os.WriteFile(path, []byte("#include \"types.h\"\n#ifdef API_ENABLED\nCode selected(Code);\nCode selected(Code);\n#endif\nlong double unrelated(long double);\n"), 0600); err != nil {
		t.Fatal(err)
	}
	header, err := Parse(context.Background(), path, opts)
	if err != nil || len(header.Functions) != 1 || header.Functions[0].Signature.Result != abi.U16 {
		t.Fatalf("include/flag/requested declaration: %+v, %v", header, err)
	}
	opts.Flags = nil
	if _, err := Parse(context.Background(), path, opts); err == nil || !strings.Contains(err.Error(), "selected") {
		t.Fatal("preprocessor-disabled declaration selected:", err)
	}
}
