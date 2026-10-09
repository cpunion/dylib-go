//go:build llgo && cgo && (linux || darwin || windows)

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/compiler/clang"
)

func TestGeneratedDirectDeclarations(t *testing.T) {
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
	names := []string{"s8", "u8", "s16", "u16", "s32", "u32", "s64", "u64", "f32", "f64", "truth", "pointer", "zero", "empty", "mixed"}
	header, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := header.LLGoSource("main", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatalf("generated direct declarations are stale:\n%s", fresh)
	}
	object := filepath.Join(t.TempDir(), "exports.o")
	flags := []string{"--target=" + header.Target.Triple, "-O0", "-c", "-fno-stack-protector", "testdata/exports.c", "-o", object}
	if runtime.GOOS != "windows" {
		flags = append(flags, "-fPIC")
	}
	if data, err := exec.Command(compiler, flags...).CombinedOutput(); err != nil {
		t.Fatalf("direct C producer: %v\n%s", err, data)
	}
	if got, err := call(object); err != nil || got != 42 {
		t.Fatal("direct library example:", got, err)
	}
	session := dylib.New(dylib.Options{})
	defer session.Close()
	if err := session.Load(object); err != nil {
		t.Fatal(err)
	}
	b, err := NewBindings(session)
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, got, want any, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: %+v, %v; want %+v", name, got, err, want)
		}
	}
	s8, err := b.S8(-128)
	check("s8", s8, int8(-127), err)
	u8, err := b.U8(255)
	check("u8", u8, uint8(254), err)
	s16, err := b.S16(-32768)
	check("s16", s16, int16(-32767), err)
	u16, err := b.U16(65535)
	check("u16", u16, uint16(65534), err)
	s32, err := b.S32(-2147483648)
	check("s32", s32, int32(-2147483647), err)
	u32, err := b.U32(4294967295)
	check("u32", u32, uint32(4294967294), err)
	s64, err := b.S64(-9223372036854775808)
	check("s64", s64, int64(-9223372036854775807), err)
	u64, err := b.U64(18446744073709551615)
	check("u64", u64, uint64(18446744073709551614), err)
	f32, err := b.F32(40.5)
	check("f32", f32, float32(42), err)
	f64, err := b.F64(40.5)
	check("f64", f64, float64(42), err)
	truth, err := b.Truth(false)
	check("truth false", truth, true, err)
	truth, err = b.Truth(true)
	check("truth true", truth, false, err)
	zero, err := b.Zero()
	check("zero", zero, int32(42), err)
	if err := b.Empty(); err != nil {
		t.Fatal(err)
	}
	pointer, err := b.Pointer(nil)
	check("null pointer", pointer, unsafe.Pointer(nil), err)
	symbol, err := session.Resolve("zero")
	if err != nil {
		t.Fatal(err)
	}
	err = symbol.WithAddress(func(address uintptr) error {
		want := unsafe.Pointer(address)
		got, err := b.Pointer(want)
		check("native pointer", got, want, err)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Zero(); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("closed binding:", err)
	}
	if err := b.Empty(); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("closed void binding:", err)
	}
	var absent *Bindings
	var unbound Bindings
	if _, err := unbound.Zero(); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("zero binding:", err)
	}
	if _, err := absent.Zero(); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("nil binding:", err)
	}
	if err := absent.Empty(); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("nil void binding:", err)
	}
	if _, err := NewBindings(nil); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("nil session:", err)
	}
}
