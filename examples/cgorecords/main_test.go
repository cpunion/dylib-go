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

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/clang"
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
	for _, tool := range []string{compiler, "llvm-ar"} {
		if _, err := exec.LookPath(tool); err != nil {
			if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
				t.Fatal(err)
			}
			t.Skip(err)
		}
	}
	return compiler
}

func TestGeneratedCgoRecords(t *testing.T) {
	compiler := compilerTool(t)
	names := []string{"echo_small", "echo_pair", "echo_outer", "echo_floats", "echo_flags", "small_factory", "apply_small", "outer_factory", "apply_outer", "record_variable", "data_pointer"}
	header, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	for i, function := range header.Functions {
		if function.Name == "record_variable" {
			header.Functions[i], err = function.WithTail(abi.I8, abi.F32)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	checkFresh := func(header *clang.Header, binding, filename string) {
		t.Helper()
		fresh, err := header.CgoSource("main", binding)
		if err != nil {
			t.Fatal(err)
		}
		committed, err := os.ReadFile(fmt.Sprintf("%s_%s_%s.go", filename, runtime.GOOS, runtime.GOARCH))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
			t.Fatalf("generated %s bindings are stale", binding)
		}
	}
	checkFresh(header, "Bindings", "generated")
	packed, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: header.Target.Triple, Flags: []string{"-fpack-struct=1"}, Functions: []string{"echo_pair"}})
	if err != nil {
		t.Fatal(err)
	}
	checkFresh(packed, "PackedBindings", "packed")
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			dir := t.TempDir()
			object, archive := filepath.Join(dir, "exports.o"), filepath.Join(dir, "exports.a")
			compile := func(tool string, args ...string) {
				t.Helper()
				if data, err := exec.Command(tool, args...).CombinedOutput(); err != nil {
					t.Fatalf("%s: %v\n%s", tool, err, data)
				}
			}
			flags := []string{"--target=" + header.Target.Triple, optimization, "-c", "-fno-stack-protector", "testdata/exports.c", "-o", object}
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
			if runtime.GOARCH == "386" {
				if runtime.GOOS == "linux" {
					linkFlags = append(linkFlags, "-m32")
				} else if runtime.GOOS == "windows" {
					linkFlags = append(linkFlags, "--target=i686-w64-windows-gnu")
					if root := os.Getenv("DYLIB_NATIVE_SYSROOT"); root != "" {
						linkFlags = append(linkFlags, "--sysroot="+root, "--rtlib=libgcc")
					}
				}
			}
			compile(compiler, append(linkFlags, optimization, "testdata/exports.c", "-o", library)...)
			for _, input := range []struct{ name, path string }{{"object", object}, {"archive", archive}, {"library", library}} {
				t.Run(input.name, func(t *testing.T) {
					if got, err := call(input.path); err != nil || got != 42 {
						t.Fatal("record example:", got, err)
					}
					session, err := load(input.path)
					if err != nil {
						t.Fatal(err)
					}
					defer session.Close()
					b, err := NewBindings(session)
					if err != nil {
						t.Fatal(err)
					}
					checkRecordCalls(t, session, b)
					if err := session.Close(); err != nil {
						t.Fatal(err)
					}
					if got, err := b.Echo_small(BindingsRecord0{}); got != (BindingsRecord0{}) || !errors.Is(err, dylib.ErrClosed) {
						t.Fatal("closed record binding:", got, err)
					}
				})
			}
		})
	}
}

func checkRecordCalls(t *testing.T, session *dylib.Session, b *Bindings) {
	t.Helper()
	check := func(name string, got, want any, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: %+v, %v; want %+v", name, got, err, want)
		}
	}
	small, wantSmall := BindingsRecord0{A: 20, B: 22}, BindingsRecord0{A: 21, B: 21}
	gotSmall, err := b.Echo_small(small)
	check("integer aggregate", gotSmall, wantSmall, err)
	pair, wantPair := BindingsRecord1{Tag: -128, Value: 20.5, Tail: -32767}, BindingsRecord1{Tag: -127, Value: 22, Tail: -32768}
	gotPair, err := b.Echo_pair(pair)
	check("padded/narrow record", gotPair, wantPair, err)
	data, err := b.Data_pointer()
	if err != nil || data == nil {
		t.Fatal("native address:", err)
	}
	// Native storage only: ordinary Go pointers cannot be retained by C.
	outer := BindingsRecord2{Items: [2]BindingsRecord1{pair, pair}, Values: [2][3]int32{{1, 2, 3}, {20, 22, 42}}, Next: data}
	wantOuter := BindingsRecord2{Items: [2]BindingsRecord1{wantPair, wantPair}, Values: [2][3]int32{{2, 3, 4}, {21, 23, 43}}, Next: data}
	gotOuter, err := b.Echo_outer(outer)
	check("nested/large/native address", gotOuter, wantOuter, err)
	floats := BindingsRecord3{Values: [4]float32{10.5, 11.5, 12.5, 13.5}}
	gotFloats, err := b.Echo_floats(floats)
	check("floating aggregate", gotFloats, BindingsRecord3{Values: [4]float32{12, 13, 14, 15}}, err)
	flags := BindingsRecord4{Truth: true, U8: 255, U16: 65535, S64: -9223372036854775808, U64: ^uint64(0)}
	gotFlags, err := b.Echo_flags(flags)
	check("boolean/integer widths", gotFlags, BindingsRecord4{Truth: false, U8: 254, U16: 65534, S64: -9223372036854775807, U64: ^uint64(0) - 1}, err)
	flags.Truth = false
	gotFlags, err = b.Echo_flags(flags)
	check("false boolean", gotFlags.Truth, true, err)
	variable, err := b.Record_variable(small, -8, 10)
	check("record variadic prefix/result", variable, BindingsRecord0{A: 12, B: 32}, err)
	// Keep code/data in the image alive throughout native entry use.
	factory, err := session.Resolve("small_factory")
	if err != nil {
		t.Fatal(err)
	}
	err = factory.WithAddress(func(_ uintptr) error {
		entry, err := b.Small_factory()
		if err != nil || entry == nil {
			t.Fatal("record factory:", err)
		}
		got, err := b.Apply_small(entry, small)
		check("native record prototype", got, wantSmall, err)
		outerEntry, err := b.Outer_factory()
		if err != nil || outerEntry == nil {
			t.Fatal("large record factory:", err)
		}
		large, err := b.Apply_outer(outerEntry, outer)
		check("native large record prototype", large, wantOuter, err)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCgoRecordValidation(t *testing.T) {
	if _, err := NewBindings(nil); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("nil session:", err)
	}
	var absent *Bindings
	var unbound Bindings
	for _, b := range []*Bindings{absent, &unbound} {
		if got, err := b.Echo_small(BindingsRecord0{}); got != (BindingsRecord0{}) || !errors.Is(err, dylib.ErrClosed) {
			t.Fatal("nil/zero record binding:", got, err)
		}
	}
	empty := dylib.New(dylib.Options{})
	defer empty.Close()
	if _, err := NewPackedBindings(empty); err == nil || !strings.Contains(err.Error(), "compiler/cgo layout mismatch") {
		t.Fatal("packed C layout must fail before symbol resolution:", err)
	}
	saved := _BindingsDeclarations.Target
	defer func() { _BindingsDeclarations.Target = saved }()
	_BindingsDeclarations.Target.PointerSize = 1
	if _, err := NewBindings(empty); err == nil || !strings.Contains(err.Error(), "declaration target does not match") {
		t.Fatal("target must fail before symbol resolution:", err)
	}
}
