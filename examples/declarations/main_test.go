//go:build libffi && cgo && (linux || darwin || windows)

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/clang"
)

func TestGeneratedDeclarationsAndNativeCalls(t *testing.T) {
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = os.Getenv("CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	if _, err := exec.LookPath(compiler); err != nil {
		if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	header, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{
		Compiler: compiler, Target: Declarations.Target.Triple,
		Functions: []string{"add", "mixed", "truth", "pointer", "variable", "sum_pair", "echo_pair", "swap_pair"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := header.GoSource("main", "Declarations")
	if err != nil {
		t.Fatal(err)
	}
	fresh = bytes.Replace(fresh, []byte("\npackage main"), []byte("\n//go:build libffi && cgo\n\npackage main"), 1)
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	// Windows Git checkouts may use CRLF; compare canonical source bytes.
	committed = bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(fresh, committed) {
		t.Fatalf("generated declarations are stale:\ncommitted:\n%s\nfresh:\n%s", committed, fresh)
	}
	object := filepath.Join(t.TempDir(), "exports.o")
	flags := []string{"--target=" + Declarations.Target.Triple, "-c", "-fno-stack-protector", "testdata/exports.c", "-o", object}
	if runtime.GOOS != "windows" {
		flags = append(flags, "-fPIC")
	}
	if data, err := exec.Command(compiler, flags...).CombinedOutput(); err != nil {
		t.Fatalf("C producer: %v\n%s", err, data)
	}
	if got, err := call(object); err != nil || got != abi.Int32(42) {
		t.Fatalf("README generated declaration example: %+v, %v", got, err)
	}
	session := dylib.New(dylib.Options{})
	defer session.Close()
	if err := session.Load(object); err != nil {
		t.Fatal(err)
	}
	if err := session.Link(); err != nil {
		t.Fatal(err)
	}
	invoke := func(name string, args []abi.Value, want abi.Value, tail ...abi.Type) {
		t.Helper()
		declaration, err := Declarations.ForHost(name)
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
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("generated %s: %+v, %v; want %+v", name, got, err, want)
		}
	}
	invoke("mixed", []abi.Value{abi.Float32(20.5), abi.Float64(21.5)}, abi.Float64(42))
	invoke("truth", []abi.Value{abi.Boolean(false)}, abi.Boolean(true))
	invoke("variable", []abi.Value{abi.Int32(20), abi.Int8(10), abi.Float32(12)}, abi.Int32(42), abi.I8, abi.F32)
	pair, err := Declarations.LookupRecord("Pair")
	if err != nil {
		t.Fatal(err)
	}
	value, err := abi.StructValue(pair.Description, abi.Int32(20), abi.Int32(22))
	if err != nil {
		t.Fatal(err)
	}
	invoke("sum_pair", []abi.Value{value}, abi.Int32(42))
	invoke("echo_pair", []abi.Value{value}, value)
	declaration, err := Declarations.ForHost("swap_pair")
	if err != nil {
		t.Fatal(err)
	}
	function, err := session.Bind(declaration.Symbol, declaration.Signature)
	if err != nil {
		t.Fatal(err)
	}
	got, err := function.Call(abi.AddressOf(&value))
	if err != nil || got.Pointee != &value || value.Aggregate.Fields[0] != abi.Int32(22) || value.Aggregate.Fields[1] != abi.Int32(20) {
		t.Fatalf("generated struct pointer mutation/identity: %+v, %v", got, err)
	}
	datum, err := session.Resolve("datum")
	if err != nil {
		t.Fatal(err)
	}
	if err := datum.WithAddress(func(address uintptr) error {
		invoke("pointer", []abi.Value{abi.Ptr(address)}, abi.Ptr(address))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
