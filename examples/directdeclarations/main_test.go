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
	"strings"
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
	names := []string{"s8", "u8", "s16", "u16", "s32", "u32", "s64", "u64", "f32", "f64", "truth", "pointer", "zero", "empty", "mixed", "echo_pair", "mutate_pair", "echo_outer", "echo_floats", "echo_small", "echo_mixed"}
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
	flags := []string{"--target=" + header.Target.Triple, "-O0", "-c", "-fno-stack-protector", "testdata/exports.c"}
	if runtime.GOOS != "windows" {
		flags = append(flags, "-fPIC")
	}
	if data, err := exec.Command(compiler, append(flags, "-o", object)...).CombinedOutput(); err != nil {
		t.Fatalf("direct C producer: %v\n%s", err, data)
	}
	if got, err := call(object); err != nil || got != 42 {
		t.Fatal("direct library example:", got, err)
	}
	session, err := load(object)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
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
	pair := BindingsRecord0{Tag: 1, Value: 20.5, Tail: 2}
	echoed, err := b.Echo_pair(pair)
	check("padded record", echoed, pair, err)
	mutated, err := b.Mutate_pair(&pair)
	if err != nil || mutated != &pair || pair != (BindingsRecord0{Tag: 2, Value: 21.5, Tail: 3}) {
		t.Fatal("typed record pointer:", pair, mutated, err)
	}
	outer := BindingsRecord1{Items: [2]BindingsRecord0{pair, pair}, Values: [2][3]int32{{1, 2, 3}, {20, 22, 42}}, Next: unsafe.Pointer(&pair)}
	echoedOuter, err := b.Echo_outer(outer)
	check("nested arrays/large return", echoedOuter, outer, err)
	floats := BindingsRecord2{Values: [4]float32{10.5, 11.5, 12.5, 13.5}}
	echoedFloats, err := b.Echo_floats(floats)
	check("floating-point aggregate", echoedFloats, floats, err)
	small := BindingsRecord3{A: 20, B: 22}
	echoedSmall, err := b.Echo_small(small)
	check("integer aggregate", echoedSmall, small, err)
	mixed := BindingsRecord4{Value: 20.5, Tag: 22}
	echoedMixed, err := b.Echo_mixed(mixed)
	check("mixed register aggregate", echoedMixed, mixed, err)
	// A separately generated packed binding must fail before symbol resolution.
	packed, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: header.Target.Triple, Flags: []string{"-fpack-struct=1"}, Functions: []string{"echo_pair"}})
	if err != nil {
		t.Fatal(err)
	}
	packedSource, err := packed.LLGoSource("main", "PackedBindings")
	if err != nil {
		t.Fatal(err)
	}
	packedCommitted, err := os.ReadFile(fmt.Sprintf("packed_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(packedSource, bytes.ReplaceAll(packedCommitted, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("packed declarations are stale")
	}
	packedObject := filepath.Join(t.TempDir(), "packed.o")
	packedFlags := append(append([]string(nil), flags...), "-fpack-struct=1", "-o", packedObject)
	if data, err := exec.Command(compiler, packedFlags...).CombinedOutput(); err != nil {
		t.Fatalf("packed producer: %v\n%s", err, data)
	}
	packedSession := dylib.New(dylib.Options{})
	defer packedSession.Close()
	if _, err := NewPackedBindings(packedSession); err == nil || !strings.Contains(err.Error(), "layout mismatch") {
		t.Fatal("layout must be checked before resolving a missing symbol:", err)
	}
	if err := packedSession.Load(packedObject); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPackedBindings(packedSession); err == nil || !strings.Contains(err.Error(), "layout mismatch") {
		t.Fatal("compiler/llgo mismatch:", err)
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
