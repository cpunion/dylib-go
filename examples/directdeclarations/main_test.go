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
	"github.com/cpunion/dylib-go/abi"
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
	names := []string{"s8", "u8", "s16", "u16", "s32", "u32", "s64", "u64", "f32", "f64", "truth", "pointer", "zero", "empty", "mixed", "echo_pair", "mutate_pair", "echo_outer", "echo_floats", "echo_small", "echo_mixed", "adder_factory", "apply_adder", "apply_anonymous", "apply_decayed", "small_factory", "apply_small", "outer_factory", "apply_outer", "mutator_factory", "apply_mutator"}
	names = append(names, "variable", "promotions", "empty_variable", "mixed_variable", "void_variable", "data_pointer", "many_variable", "record_variable", "variadic_factory", "forward_variadic")
	header, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	tails := map[string][]abi.Type{
		"variable":        {abi.I8, abi.F32},
		"promotions":      {abi.I8, abi.U8, abi.I16, abi.U16, abi.F32, abi.Bool, abi.Bool, abi.I64, abi.U64, abi.Pointer},
		"mixed_variable":  {abi.I8, abi.F32},
		"void_variable":   {abi.I16},
		"record_variable": {abi.I32, abi.F64},
	}
	for range 10 {
		tails["many_variable"] = append(tails["many_variable"], abi.I64, abi.F64)
	}
	for i, function := range header.Functions {
		if tail, ok := tails[function.Name]; ok {
			header.Functions[i], err = function.WithTail(tail...)
			if err != nil {
				t.Fatal(err)
			}
		}
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
	checkDirectVariadicCalls(t, session, b)
	// Keep the producer image leased while using returned native entries.
	factory, err := session.Resolve("adder_factory")
	if err != nil {
		t.Fatal(err)
	}
	err = factory.WithAddress(func(_ uintptr) error {
		adder, err := b.Adder_factory()
		if err != nil || adder == nil {
			t.Fatal("native factory:", err)
		}
		check("native scalar entry", adder(20, 22), int32(42), nil)
		for _, apply := range []func(BindingsCallback0, int32, int32) (int32, error){b.Apply_adder, b.Apply_anonymous, b.Apply_decayed} {
			got, err := apply(adder, 20, 22)
			check("typed scalar callback", got, int32(42), err)
		}
		smallEntry, err := b.Small_factory()
		if err != nil || smallEntry == nil {
			t.Fatal("small factory:", err)
		}
		check("native record entry", smallEntry(small), small, nil)
		gotSmall, err := b.Apply_small(smallEntry, small)
		check("typed record callback", gotSmall, small, err)
		outerEntry, err := b.Outer_factory()
		if err != nil || outerEntry == nil {
			t.Fatal("large factory:", err)
		}
		check("native large entry", outerEntry(outer), outer, nil)
		gotOuter, err := b.Apply_outer(outerEntry, outer)
		check("typed large callback", gotOuter, outer, err)
		mutator, err := b.Mutator_factory()
		if err != nil || mutator == nil {
			t.Fatal("pointer factory:", err)
		}
		gotPointer, err := b.Apply_mutator(mutator, &pair)
		if err != nil || gotPointer != &pair || pair != (BindingsRecord0{Tag: 3, Value: 22.5, Tail: 4}) {
			t.Fatal("typed pointer callback:", pair, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Noncapturing C literals reenter the already registered Go calling thread.
	// Foreign-thread entries and captures need separately owned callback adapters.
	localAdder := BindingsCallback0(func(a, b int32) int32 { return a + b })
	localSum, err := b.Apply_adder(localAdder, 20, 22)
	check("Go to C to local Go entry", localSum, int32(42), err)
	localRecord := BindingsCallback1(func(value BindingsRecord3) BindingsRecord3 {
		value.A++
		value.B--
		return value
	})
	localPair, err := b.Apply_small(localRecord, small)
	check("local Go record entry", localPair, BindingsRecord3{A: 21, B: 21}, err)
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
	if _, err := b.Variable(20, 10, 12); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("closed variadic binding:", err)
	}
	if entry, err := b.Variadic_factory(); entry != nil || !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("closed variadic factory:", err)
	}
	if entry, err := b.Adder_factory(); entry != nil || !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("closed native factory:", err)
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

func checkDirectVariadicCalls(t *testing.T, session *dylib.Session, b *Bindings) {
	t.Helper()
	check := func(name string, got, want any, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: %+v, %v; want %+v", name, got, err, want)
		}
	}
	small := BindingsRecord3{A: 20, B: 22}
	variable, err := b.Variable(20, 10, 12)
	check("variadic narrow/float promotion", variable, int32(42), err)
	promoted, err := b.Promotions(7, -8, 255, -300, 65535, 20.5, true, false, -9, ^uint64(0), nil)
	check("all C default promotions", promoted, int32(42), err)
	emptyVariable, err := b.Empty_variable(42)
	check("empty variadic tail", emptyVariable, int32(42), err)
	mixedVariable, err := b.Mixed_variable(3, 20, 7, 12)
	check("fixed float/variadic double", mixedVariable, float64(42), err)
	many, err := b.Many_variable(10, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2)
	check("variadic integer/FP register spill", many, float64(30), err)
	recordVariable, err := b.Record_variable(small, 1, -1)
	check("variadic record prefix/result", recordVariable, BindingsRecord3{A: 21, B: 21}, err)
	data, err := b.Data_pointer()
	if err != nil || data == nil {
		t.Fatal("native data pointer:", err)
	}
	if err := b.Void_variable(data, 42); err != nil || *(*int32)(data) != 42 {
		t.Fatal("void variadic/pointer mutation:", err)
	}
	variadicFactory, err := session.Resolve("variadic_factory")
	if err != nil {
		t.Fatal(err)
	}
	err = variadicFactory.WithAddress(func(_ uintptr) error {
		entry, err := b.Variadic_factory()
		if err != nil || entry == nil {
			t.Fatal("variadic native factory:", err)
		}
		check("returned variadic native entry", entry(20, int32(10), float64(12)), int32(42), nil)
		forwarded, err := b.Forward_variadic(entry)
		check("variadic native pointer forwarding", forwarded, int32(42), err)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDirectVariadicArchivesAndLibraries(t *testing.T) {
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = os.Getenv("CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	for _, tool := range []string{compiler, "llvm-ar"} {
		if _, err := exec.LookPath(tool); err != nil {
			if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
				t.Fatal(err)
			}
			t.Skip(err)
		}
	}
	dir := t.TempDir()
	object := filepath.Join(dir, "exports.o")
	archive := filepath.Join(dir, "exports.a")
	compile := func(tool string, args ...string) {
		t.Helper()
		if data, err := exec.Command(tool, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", tool, err, data)
		}
	}
	flags := []string{"--target=" + _BindingsDeclarations.Target.Triple, "-O0", "-c", "-fno-stack-protector", "testdata/exports.c", "-o", object}
	if runtime.GOOS != "windows" {
		flags = append(flags, "-fPIC")
	}
	compile(compiler, flags...)
	compile("llvm-ar", "rcs", archive, object)
	library, linkFlags := filepath.Join(dir, "exports.so"), []string{"-shared", "-fPIC"}
	switch runtime.GOOS {
	case "darwin":
		library, linkFlags = filepath.Join(dir, "exports.dylib"), []string{"-dynamiclib"}
	case "windows":
		library, linkFlags = filepath.Join(dir, "exports.dll"), []string{"-shared", "-Wl,--export-all-symbols"}
	}
	compile(compiler, append(linkFlags, "-O0", "testdata/exports.c", "-o", library)...)
	for _, input := range []struct{ name, path string }{{"archive", archive}, {"library", library}} {
		t.Run(input.name, func(t *testing.T) {
			session, err := load(input.path)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			bindings, err := NewBindings(session)
			if err != nil {
				t.Fatal(err)
			}
			checkDirectVariadicCalls(t, session, bindings)
		})
	}
}
