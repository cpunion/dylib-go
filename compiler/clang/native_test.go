//go:build libffi && cgo && (linux || darwin || windows)

package clang

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
)

func TestNativeDeclaredCallsAndVariadics(t *testing.T) {
	compiler := compilerTool(t)
	prototypes := "int add(int,int);\nlong wide(long,unsigned long);\n_Bool truth(_Bool);\ndouble mixed(float,double);\nvoid *pointer(void*);\nchar character(char);\nint variable(int,...);\n"
	names := []string{"add", "wide", "truth", "mixed", "pointer", "character", "variable"}
	if runtime.GOOS == "windows" && runtime.GOARCH == "386" {
		prototypes += "int __attribute__((stdcall)) std_add(int,int);\nint __attribute__((fastcall)) fast_add(int,int);\n"
		names = append(names, "std_add", "fast_add")
	}
	path := headerFile(t, prototypes)
	opts := Options{Compiler: compiler, Functions: names}
	header, err := Parse(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source, object := filepath.Join(dir, "exports.c"), filepath.Join(dir, "exports.o")
	code := "int datum=42;\nint add(int a,int b){return a+b;}\nlong wide(long a,unsigned long b){return a+b;}\n_Bool truth(_Bool x){return !x;}\ndouble mixed(float a,double b){return a+b;}\nvoid *pointer(void*x){return x;}\nchar character(char x){return x;}\nint variable(int a,...){__builtin_va_list ap;__builtin_va_start(ap,a);int b=__builtin_va_arg(ap,int);double c=__builtin_va_arg(ap,double);__builtin_va_end(ap);return a+b+(int)c;}\n"
	if runtime.GOOS == "windows" && runtime.GOARCH == "386" {
		code += "int __attribute__((stdcall)) std_add(int a,int b){return a+b;}\nint __attribute__((fastcall)) fast_add(int a,int b){return a+b;}\n"
	}
	if err := os.WriteFile(source, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	flags := []string{"--target=" + nativeTriple(), "-c", "-O0", "-fno-stack-protector", source, "-o", object}
	if runtime.GOOS != "windows" {
		flags = append(flags, "-fPIC")
	}
	if data, err := exec.Command(compiler, flags...).CombinedOutput(); err != nil {
		t.Fatalf("C producer: %v\n%s", err, data)
	}
	session := dylib.New(dylib.Options{})
	defer session.Close()
	if err := session.Load(object); err != nil {
		t.Fatal(err)
	}
	if err := session.Link(); err != nil {
		t.Fatal(err)
	}
	call := func(name string, args []abi.Value, want abi.Value, tail ...abi.Type) {
		t.Helper()
		declaration, err := header.ForHost(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(tail) != 0 {
			declaration, err = declaration.WithTail(tail...)
			if err != nil {
				t.Fatal(err)
			}
		}
		fn, err := session.Bind(declaration.Symbol, declaration.Signature)
		if err != nil {
			t.Fatal(err)
		}
		got, err := fn.Call(args...)
		if err != nil || got != want {
			t.Fatalf("declared %s: %+v, %v; want %+v", name, got, err, want)
		}
	}
	call("add", []abi.Value{abi.Int32(20), abi.Int32(22)}, abi.Int32(42))
	if runtime.GOOS == "windows" && runtime.GOARCH == "386" {
		for _, name := range []string{"std_add", "fast_add"} {
			call(name, []abi.Value{abi.Int32(20), abi.Int32(22)}, abi.Int32(42))
		}
	}
	wide, _ := header.Lookup("wide")
	if wide.Signature.Result == abi.I64 {
		call("wide", []abi.Value{abi.Int64(20), abi.Uint64(22)}, abi.Int64(42))
	} else {
		call("wide", []abi.Value{abi.Int32(20), abi.Uint32(22)}, abi.Int32(42))
	}
	call("truth", []abi.Value{abi.Boolean(false)}, abi.Boolean(true))
	call("mixed", []abi.Value{abi.Float32(20.5), abi.Float64(21.5)}, abi.Float64(42))
	character, _ := header.Lookup("character")
	if character.Signature.Result == abi.U8 {
		call("character", []abi.Value{abi.Uint8(200)}, abi.Uint8(200))
	} else {
		call("character", []abi.Value{abi.Int8(-56)}, abi.Int8(-56))
	}
	call("variable", []abi.Value{abi.Int32(20), abi.Int8(10), abi.Float32(12)}, abi.Int32(42), abi.I8, abi.F32)
	datum, err := session.Resolve("datum")
	if err != nil {
		t.Fatal(err)
	}
	if err := datum.WithAddress(func(address uintptr) error {
		call("pointer", []abi.Value{abi.Ptr(address)}, abi.Ptr(address))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
