package clang

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPackedBitfieldBridgeMSVCAndMinGW(t *testing.T) {
	testPackedBitfieldBridgeMSVCAndMinGW(t, `typedef struct __attribute__((packed)) { unsigned a:3; signed b:5; _Bool on:1; unsigned long long big:40; double real; } Bits; Bits echo(Bits);`)
}

func TestPackedPaddingBridgeMSVCAndMinGW(t *testing.T) {
	testPackedBitfieldBridgeMSVCAndMinGW(t, `typedef struct __attribute__((packed)) { unsigned :3; unsigned a:5; unsigned :0; signed b:7; unsigned :1; _Bool on:1; double real; unsigned :5; } Bits; Bits echo(Bits);`)
}

func TestPackedPaddingWideBoundaries(t *testing.T) {
	testPackedBitfieldBridgeMSVCAndMinGW(t, `typedef struct __attribute__((packed)) { unsigned char first:1; unsigned long long :0; unsigned :3; signed value:5; unsigned long long :0; void *pointer; double items[2]; unsigned long long :0; } Bits; Bits echo(Bits);`)
}

func testPackedBitfieldBridgeMSVCAndMinGW(t *testing.T, source string) {
	t.Helper()
	path := headerFile(t, source)
	compiler := compilerTool(t)
	for _, target := range targets {
		if target.os != "windows" {
			continue
		}
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: []string{"echo"}})
			if err != nil {
				t.Fatal(err)
			}
			r := h.Records[0]
			source, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			text := string(source)
			start := strings.Index(text, "/*") + 2
			end := strings.Index(text[start:], "*/") + start
			name := fmt.Sprintf("dylib_go_%x_record0", "Bindings")
			var probe strings.Builder
			probe.WriteString(text[start:end])
			fmt.Fprintf(&probe, "\n_Static_assert(sizeof(%s)==%d,\"size\");\n_Static_assert(_Alignof(%s)==%d,\"alignment\");\n", name, r.Layout.Size, name, r.Layout.Alignment)
			for i, offset := range r.Layout.Offsets {
				if r.bitfieldAt(i) == nil {
					fmt.Fprintf(&probe, "_Static_assert(offsetof(%s,f%d)==%d,\"offset\");\n", name, i, offset)
				}
			}
			for _, f := range r.Bitfields {
				fmt.Fprintf(&probe, "const %s dylib_bitfield_%d = {.f%d=%s};\n", name, f.Position, f.Position, bitfieldValue(r.Description.Fields[f.Position].Type.Type, f.Width))
			}
			// The host bridge may use MinGW while the producer uses MSVC.
			for _, triple := range []string{target.triple, strings.Replace(target.triple, "pc-windows-msvc", "w64-windows-gnu", 1)} {
				object := filepath.Join(t.TempDir(), "probe.o")
				if _, err := runCompiler(context.Background(), compiler, []string{"--target=" + triple, "-ffreestanding", "-x", "c", "-std=c17", "-c", "-o", object, "-"}, probe.String(), maxDiagnostics); err != nil {
					t.Fatal(triple, err)
				}
				data, err := os.ReadFile(object)
				if err != nil {
					t.Fatal(err)
				}
				symbols, err := readBitfieldSymbols(data, "windows")
				if err != nil {
					t.Fatal(err)
				}
				for _, f := range r.Bitfields {
					value, err := symbols.value(fmt.Sprintf("dylib_bitfield_%d", f.Position), r.Layout.Size)
					want := make([]byte, r.Layout.Size)
					copy(want[r.Layout.Offsets[f.Position]:], f.Mask)
					if err != nil || !bytes.Equal(value, want) {
						t.Fatal(triple, f.Position, value, want, err)
					}
				}
			}
		})
	}
}

func TestBitfieldRecordsAcrossTargets(t *testing.T) {
	path := headerFile(t, `typedef struct { unsigned flags:3; signed delta:5; _Bool active:1; unsigned value; } Bits;
typedef struct { Bits items[2]; } Outer;
Bits echo(Bits); Outer nest(Outer); Bits *pointer(Bits *);`)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"echo", "nest", "pointer"}})
			if err != nil {
				t.Fatal(err)
			}
			r, err := h.LookupRecord("Bits")
			if err != nil || len(r.Bitfields) != 3 {
				t.Fatal(r, err)
			}
			active, value, size := uint64(1), uint64(4), uint64(8)
			if target.os == "windows" {
				active, value, size = 4, 8, 12
			}
			if r.Layout.Size != size || !reflect.DeepEqual(r.Layout.Offsets, []uint64{0, 0, active, value}) || !reflect.DeepEqual(r.Bitfields, []Bitfield{{Position: 0, Width: 3, Mask: []byte{7}}, {Position: 1, Width: 5, Mask: []byte{248}}, {Position: 2, Width: 1, Mask: []byte{1}}}) {
				t.Fatal("compiler bit masks:", r)
			}
			r.Bitfields[0].Mask[0] = 255
			fresh, _ := h.LookupRecord("Bits")
			if fresh.Bitfields[0].Mask[0] != 7 {
				t.Fatal("mask snapshot is shared")
			}
			source, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"Bitfields:", "uint32_t f0:3;", "int32_t f1:5;", "_Bool f2:1;", "_mask(0,UINT64_MAX)", "_from_transport(p0)"} {
				if !strings.Contains(string(source), marker) {
					t.Fatal("missing generated bitfield", marker)
				}
			}
			if target.arch != "386" {
				if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
					t.Fatal("direct bitfields accepted")
				}
			}
			if h.Target.CheckHost() == nil {
				for _, name := range []string{"echo", "nest", "pointer"} {
					if _, err := h.ForHost(name); err == nil || !strings.Contains(err.Error(), "compiled C adapter") {
						t.Fatal(name, err)
					}
				}
			}
		})
	}
}

func TestBitfieldMaskValidation(t *testing.T) {
	h, err := Parse(context.Background(), headerFile(t, `typedef struct { unsigned flags:3; signed delta:5; } Bits; Bits echo(Bits);`), Options{Compiler: compilerTool(t), Functions: []string{"echo"}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := h.LookupRecord("Bits")
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []func(*Record){
		func(r *Record) { r.Bitfields[0].Mask = nil },
		func(r *Record) { r.Bitfields[0].Mask = []byte{11} },
		func(r *Record) { r.Bitfields[0].Width = 65 },
		func(r *Record) { r.Bitfields[0].Position = 20 },
		func(r *Record) { r.Bitfields[1].Position = -1 },
		func(r *Record) { r.Bitfields[1].Position = 0 },
		func(r *Record) { r.Bitfields[1].Mask = []byte{31} },
		func(r *Record) { r.Bitfields[0].Mask = make([]byte, 10) },
		func(r *Record) { r.Layout.Offsets[1] = r.Layout.Size },
	} {
		copy := r
		copy.Bitfields = cloneBitfields(r.Bitfields)
		copy.Layout.Offsets = append([]uint64(nil), r.Layout.Offsets...)
		mutation(&copy)
		if err := validRecordLayout(copy); err == nil {
			t.Fatal("invalid bitfield accepted", copy)
		}
	}
	for _, os := range []string{"linux", "darwin", "windows", "other"} {
		if _, err := readBitfieldSymbols([]byte("broken"), os); err == nil {
			t.Fatal("invalid probe accepted", os)
		}
	}
	for _, name := range []string{"missing", "short"} {
		if _, err := (bitfieldSymbols{"short": {1}}).value(name, 8); err == nil {
			t.Fatal("invalid symbol accepted", name)
		}
	}
}
