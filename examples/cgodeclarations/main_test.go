//go:build cgo && (linux || darwin || windows)

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
	"strings"
	"testing"
	"unsafe"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/clang"
)

func TestGeneratedCgoDeclarations(t *testing.T) {
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
	names := []string{"s8", "u8", "s16", "u16", "s32", "u32", "s64", "u64", "f32", "f64", "truth", "pointer", "zero", "empty", "mixed", "variable", "promotions", "empty_variable", "mixed_variable", "void_variable", "adder_factory", "apply_adder", "variadic_factory", "forward_variadic", "data_pointer"}
	header, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	tails := map[string][]abi.Type{
		"variable":       {abi.I8, abi.F32},
		"promotions":     {abi.I8, abi.U8, abi.I16, abi.U16, abi.F32, abi.Bool, abi.I64, abi.U64, abi.Pointer},
		"mixed_variable": {abi.I8, abi.F32},
		"void_variable":  {abi.I16},
	}
	for i, function := range header.Functions {
		if tail, ok := tails[function.Name]; ok {
			header.Functions[i], err = function.WithTail(tail...)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	fresh, err := header.CgoSource("main", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(fmt.Sprintf("generated_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatalf("generated cgo declarations are stale:\n%s", fresh)
	}
	object := filepath.Join(t.TempDir(), "exports.o")
	flags := []string{"--target=" + header.Target.Triple, "-O0", "-c", "-fno-stack-protector", "testdata/exports.c", "-o", object}
	if runtime.GOOS != "windows" {
		flags = append(flags, "-fPIC")
	}
	if data, err := exec.Command(compiler, flags...).CombinedOutput(); err != nil {
		t.Fatalf("C producer: %v\n%s", err, data)
	}
	if got, err := call(object); err != nil || got != 42 {
		t.Fatal("cgo library example:", got, err)
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
	mixed, err := b.Mixed(10, 20.5, 1.5, 10)
	check("mixed", mixed, float64(42), err)
	pointer, err := b.Pointer(nil)
	check("null pointer", pointer, unsafe.Pointer(nil), err)
	variable, err := b.Variable(20, 10, 12)
	check("variadic int8/float32", variable, int32(42), err)
	emptyVariable, err := b.Empty_variable(42)
	check("empty variadic tail", emptyVariable, int32(42), err)
	mixedVariable, err := b.Mixed_variable(20, 7, 10, 12)
	check("fixed float/variadic float", mixedVariable, int32(42), err)
	datum, err := session.Resolve("datum")
	if err != nil {
		t.Fatal(err)
	}
	err = datum.WithAddress(func(_ uintptr) error {
		want, err := b.Data_pointer()
		if err != nil || want == nil {
			t.Fatal("native data pointer:", err)
		}
		got, err := b.Pointer(want)
		check("native pointer", got, want, err)
		promoted, err := b.Promotions(7, -8, 250, -300, 60000, 11.5, true, -9, 18446744073709551614, want)
		check("all default promotions/native pointer", promoted, int32(42), err)
		if err := b.Void_variable(7, 41); err != nil {
			return err
		}
		check("variadic void mutation", *(*int32)(want), int32(41), nil)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Factory calls lease the factory only. Keep its image alive during later calls.
	factory, err := session.Resolve("adder_factory")
	if err != nil {
		t.Fatal(err)
	}
	err = factory.WithAddress(func(_ uintptr) error {
		entry, err := b.Adder_factory()
		if err != nil || entry == nil {
			t.Fatal("native factory:", err)
		}
		got, err := b.Apply_adder(entry, 20, 22)
		check("native function pointer", got, int32(42), err)
		variadicEntry, err := b.Variadic_factory()
		if err != nil || variadicEntry == nil {
			t.Fatal("variadic native factory:", err)
		}
		got, err = b.Forward_variadic(variadicEntry, 20)
		check("forwarded variadic prototype", got, int32(42), err)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Zero(); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("closed scalar binding:", err)
	}
	if entry, err := b.Adder_factory(); entry != nil || !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("closed factory:", err)
	}
	if err := b.Empty(); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("closed void binding:", err)
	}
	var absent *Bindings
	var unbound Bindings
	for _, binding := range []*Bindings{absent, &unbound} {
		if _, err := binding.Zero(); !errors.Is(err, dylib.ErrClosed) {
			t.Fatal("nil/zero binding:", err)
		}
		if err := binding.Empty(); !errors.Is(err, dylib.ErrClosed) {
			t.Fatal("nil/zero void binding:", err)
		}
	}
	if _, err := NewBindings(nil); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("nil session:", err)
	}
	// Validate the target before resolving exports, even on an empty session.
	saved := _BindingsDeclarations.Target
	defer func() { _BindingsDeclarations.Target = saved }()
	_BindingsDeclarations.Target.PointerSize = 1
	emptySession := dylib.New(dylib.Options{})
	defer emptySession.Close()
	if _, err := NewBindings(emptySession); err == nil || !strings.Contains(err.Error(), "declaration target does not match") {
		t.Fatal("invalid target must fail before resolution:", err)
	}
}
