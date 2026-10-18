//go:build cgo && darwin && arm64

package plainpadding

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/clang"
)

func TestPlainPaddingCalls(t *testing.T) {
	compiler := os.Getenv("CLANG")
	if override := os.Getenv("DYLIB_LLVM_CLANG"); override != "" {
		compiler = override
	}
	if compiler == "" {
		compiler = "clang"
	}
	tool := func(name string) string {
		t.Helper()
		path, err := exec.LookPath(name)
		if err != nil {
			if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
				t.Fatal(err)
			}
			t.Skip(err)
		}
		return path
	}
	compiler, archiver := tool(compiler), tool("ar")
	names, err := os.ReadFile("testdata/functions.txt")
	if err != nil {
		t.Fatal(err)
	}
	h, err := clang.Parse(context.Background(), "testdata/exports.h", clang.Options{Compiler: compiler, Target: _BindingsDeclarations.Target.Triple, Functions: strings.Fields(string(names))})
	if err != nil {
		t.Fatal(err)
	}
	wide, err := h.LookupRecord("PlainWide")
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range h.Functions {
		switch d.Name {
		case "plain_variable":
			h.Functions[i], err = d.WithTail(abi.I8, abi.F32)
			if err != nil {
				t.Fatal(err)
			}
		case "plain_record_variable":
			// Rich signatures can describe a record in the variadic tail;
			// WithTail's scalar convenience form has no field descriptors.
			d.Signature.Args = append(d.Signature.Args, abi.Struct, abi.I8, abi.F32)
			d.Signature.ArgTypes = []abi.TypeDesc{{Type: abi.I32}, wide.Description, {Type: abi.I8}, {Type: abi.F32}}
			h.Functions[i] = d
		}
	}
	fresh, err := h.CgoSource("plainpadding", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("generated_darwin_arm64.go")
	if err != nil || !bytes.Equal(fresh, committed) {
		t.Fatal("generated plain unnamed-field bindings are stale", err)
	}
	compile := func(t *testing.T, tool string, args ...string) {
		t.Helper()
		if output, err := exec.Command(tool, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", tool, err, output)
		}
	}
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			dir := t.TempDir()
			object, archive, library := filepath.Join(dir, "plain.o"), filepath.Join(dir, "plain.a"), filepath.Join(dir, "plain.dylib")
			compile(t, compiler, optimization, "-fPIC", "-fno-stack-protector", "-c", "testdata/exports.c", "-o", object)
			compile(t, archiver, "rcs", archive, object)
			compile(t, compiler, "-dynamiclib", object, "-o", library)
			for _, input := range []struct{ name, path string }{{"object", object}, {"archive", archive}, {"library", library}} {
				t.Run(input.name, func(t *testing.T) { checkCalls(t, input.path) })
			}
		})
	}
}

func checkCalls(t *testing.T, path string) {
	t.Helper()
	if result, err := Call(path); err != nil || result != 42 {
		t.Fatal("plain unnamed-field example", result, err)
	}
	session := dylib.New(dylib.Options{ProcessSymbols: true})
	defer session.Close()
	if err := session.Load(path); err != nil {
		t.Fatal(err)
	}
	b, err := NewBindings(session)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := session.Resolve("plain_pointer")
	if err != nil {
		t.Fatal(err)
	}
	err = owner.WithAddress(func(uintptr) error {
		address, err := b.Plain_pointer()
		if err != nil || address == nil {
			t.Fatal("native storage", address, err)
		}
		check := func(t *testing.T, got, want any, err error) {
			t.Helper()
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("got %+v, %v; want %+v", got, err, want)
			}
		}
		wide := BindingsRecord0{Wide_real: 20.5, Wide_tag: 20}
		want := BindingsRecord0{Wide_real: 22, Wide_tag: 21}
		t.Run("wide", func(t *testing.T) { v, err := b.Echo_wide(wide); check(t, v, want, err) })
		t.Run("leading", func(t *testing.T) {
			v, err := b.Echo_lead(BindingsRecord1{Lead_real: 20.5})
			check(t, v, BindingsRecord1{Lead_real: 22}, err)
		})
		t.Run("trailing", func(t *testing.T) {
			v, err := b.Echo_tail(BindingsRecord2{Tail_real: 20.5})
			check(t, v, BindingsRecord2{Tail_real: 22}, err)
		})
		t.Run("floating", func(t *testing.T) {
			v, err := b.Echo_pair(BindingsRecord3{Pair_left: 20.5, Pair_right: 19.5})
			check(t, v, BindingsRecord3{Pair_left: 22, Pair_right: 22}, err)
		})
		t.Run("memory", func(t *testing.T) {
			v, err := b.Echo_mixed(BindingsRecord4{Mixed_tag: -128, Mixed_real: 20.5})
			check(t, v, BindingsRecord4{Mixed_tag: -127, Mixed_real: 22}, err)
		})
		t.Run("native_address", func(t *testing.T) {
			v, err := b.Echo_pointer(BindingsRecord5{Context: address, Pointer_real: 20.5})
			check(t, v, BindingsRecord5{Context: address, Pointer_real: 22}, err)
		})
		t.Run("multiple", func(t *testing.T) {
			v, err := b.Echo_multiple(BindingsRecord6{Multiple_tag: 20, Multiple_real: 20.5})
			check(t, v, BindingsRecord6{Multiple_tag: 21, Multiple_real: 22}, err)
		})
		t.Run("array", func(t *testing.T) {
			v, err := b.Echo_array(BindingsRecord7{Array_values: [2][2]float32{{20.5, 20.5}, {20.5, 20.5}}})
			check(t, v, BindingsRecord7{Array_values: [2][2]float32{{22, 22}, {22, 22}}}, err)
		})
		t.Run("nested", func(t *testing.T) {
			value := BindingsRecord8{Envelope_tag: 20, Items: [2][2]BindingsRecord0{{wide, wide}, {wide, wide}}, Leads: [2]BindingsRecord1{{Lead_real: 20.5}, {Lead_real: 20.5}}, Envelope_real: 20.5}
			expected := BindingsRecord8{Envelope_tag: 21, Items: [2][2]BindingsRecord0{{want, want}, {want, want}}, Leads: [2]BindingsRecord1{{Lead_real: 22}, {Lead_real: 22}}, Envelope_real: 22}
			v, err := b.Echo_envelope(value)
			check(t, v, expected, err)
		})
		t.Run("variadic_prefix", func(t *testing.T) {
			v, err := b.Plain_variable(wide, -8, 1.5)
			check(t, v, BindingsRecord0{Wide_real: 22, Wide_tag: 12}, err)
		})
		t.Run("variadic_record", func(t *testing.T) {
			v, err := b.Plain_record_variable(42, wide, -8, 1.5)
			check(t, v, BindingsRecord0{Wide_real: 22, Wide_tag: 12}, err)
		})
		t.Run("overflow", func(t *testing.T) {
			v, err := b.Plain_overflow(1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, wide)
			check(t, v, want, err)
		})
		t.Run("factory", func(t *testing.T) {
			entry, err := b.Plain_factory()
			if err != nil || entry == nil {
				t.Fatal("native entry", entry, err)
			}
			v, err := b.Plain_forward(entry, wide)
			check(t, v, want, err)
			v, err = b.Plain_forward(nil, wide)
			check(t, v, wide, err)
		})
		t.Run("pointer", func(t *testing.T) {
			if err := b.Plain_write(address, wide); err != nil {
				t.Fatal(err)
			}
			v, err := b.Plain_read(address)
			check(t, v, wide, err)
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
