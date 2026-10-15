package clang

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPackedBitfieldBridgeMSVCAndMinGW(t *testing.T) {
	testPackedBitfieldBridgeMSVCAndMinGW(t, `
typedef struct __attribute__((packed)) { unsigned a:3; signed b:5; _Bool on:1; unsigned long long big:40; double real; } PackedBits;
typedef struct __attribute__((packed)) { unsigned :3; unsigned a:5; unsigned :0; signed b:7; unsigned :1; _Bool on:1; double real; unsigned :5; } PackedPadding;
typedef struct __attribute__((packed)) { unsigned char first:1; unsigned long long :0; unsigned :3; signed value:5; unsigned long long :0; void *pointer; double items[2]; unsigned long long :0; } WideBoundaries;
typedef struct __attribute__((packed)) { unsigned a:3 __attribute__((packed)); unsigned :0; int value; } MemberPackedBoundary;
PackedBits echo_bits(PackedBits); PackedPadding echo_padding(PackedPadding); WideBoundaries echo_wide(WideBoundaries); MemberPackedBoundary echo_member(MemberPackedBoundary);
`, "echo_bits", "echo_padding", "echo_wide", "echo_member")
}

func TestAlignedBitfieldBridgeMSVCAndMinGW(t *testing.T) {
	// Share the compiler probes across these layouts; each record still gets
	// independent size, alignment, offset, and bit-mask assertions.
	testPackedBitfieldBridgeMSVCAndMinGW(t, `
typedef struct __attribute__((packed,aligned(32))) { unsigned a:3; signed b:5; _Bool on:1; double real; } PackedBits;
typedef struct __attribute__((packed,aligned(32))) { unsigned :3; unsigned a:5; unsigned :0; signed b:7; double real; } PackedPadding;
typedef struct { unsigned a:5; int value __attribute__((aligned(16))); signed b:7 __attribute__((aligned(8))); unsigned :0; char tag; } AlignedFields;
typedef struct __attribute__((packed,aligned(32))) { char tag; double values[2] __attribute__((aligned(16))); short tail; } AlignedArray;
typedef struct __attribute__((packed,aligned(32))) { unsigned packed_a:5; int packed_value __attribute__((aligned(16))); signed packed_b:7 __attribute__((aligned(8))); unsigned :0; char packed_tag; } PackedAlignedFields;
PackedBits echo_bits(PackedBits); PackedPadding echo_padding(PackedPadding); AlignedFields echo_fields(AlignedFields); AlignedArray echo_array(AlignedArray); PackedAlignedFields echo_packed_fields(PackedAlignedFields);
`, "echo_bits", "echo_padding", "echo_fields", "echo_array", "echo_packed_fields")
}

func testPackedBitfieldBridgeMSVCAndMinGW(t *testing.T, source string, functions ...string) {
	t.Helper()
	testBitfieldBridgeMSVCAndMinGW(t, source, nil, functions...)
}

func testBitfieldBridgeMSVCAndMinGW(t *testing.T, source string, flags []string, functions ...string) {
	t.Helper()
	var producers []string
	for _, target := range targets {
		if target.os == "windows" {
			producers = append(producers, target.triple, strings.Replace(target.triple, "pc-windows-msvc", "w64-windows-gnu", 1))
		}
	}
	testBitfieldBridgeTargets(t, source, flags, producers, functions...)
}

func testBitfieldBridgeTargets(t *testing.T, source string, flags, producers []string, functions ...string) {
	t.Helper()
	testCompilerBridgeTargets(t, compilerTool(t), source, flags, producers, functions...)
}

func testCompilerBridgeTargets(t *testing.T, compiler, source string, flags, producers []string, functions ...string) {
	t.Helper()
	testCompilerBridgeTargetsWithInspection(t, compiler, source, flags, producers, nil, functions...)
}

// Inspect the parsed header before compiling bridge checks, so layout, metadata
// and calling ABI verification can reuse the same producer invocations.
// An inspector also verifies its own expected record count when several
// functions share a body; the default fixtures use one record per function.
func testCompilerBridgeTargetsWithInspection(t *testing.T, compiler, source string, flags, producers []string, inspect func(*testing.T, string, *Header), functions ...string) {
	t.Helper()
	path := headerFile(t, source)
	for _, producer := range producers {
		t.Run(producer, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: producer, Flags: flags, Functions: functions})
			if errors.Is(err, errTypedefABI) {
				t.Log("compiler rejects legacy SysV typedef ABI:", err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if inspect != nil {
				inspect(t, path, h)
			}
			if inspect == nil && len(h.Records) != len(functions) {
				t.Fatalf("missing bridge cases: got %d records for %d functions", len(h.Records), len(functions))
			}
			source, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			text := string(source)
			start := strings.Index(text, "/*") + 2
			end := strings.Index(text[start:], "*/") + start
			var probe strings.Builder
			probe.WriteString(text[start:end])
			for index, r := range h.Records {
				name := fmt.Sprintf("dylib_go_%x_record%d", "Bindings", index)
				fmt.Fprintf(&probe, "\n_Static_assert(sizeof(%s)==%d,\"size\");\n_Static_assert(_Alignof(%s)==%d,\"alignment\");\n", name, r.Layout.Size, name, r.Layout.Alignment)
				for i, offset := range r.Layout.Offsets {
					if r.bitfieldAt(i) == nil {
						fmt.Fprintf(&probe, "_Static_assert(offsetof(%s,f%d)==%d,\"offset\");\n", name, i, offset)
					}
				}
				for _, f := range r.Bitfields {
					fmt.Fprintf(&probe, "const %s dylib_bitfield_%d_%d = {.f%d=%s};\n", name, index, f.Position, f.Position, bitfieldValue(r.Description.Fields[f.Position].Type.Type, f.Width))
				}
			}
			// The host bridge may use MinGW while the producer uses MSVC.
			triples := []string{producer}
			if strings.Contains(producer, "msvc") {
				triples = append(triples, strings.Replace(producer, "pc-windows-msvc", "w64-windows-gnu", 1))
			}
			for _, triple := range triples {
				object := filepath.Join(t.TempDir(), "probe.o")
				if _, err := runCompiler(context.Background(), compiler, []string{"--target=" + triple, "-ffreestanding", "-x", "c", "-std=c17", "-c", "-o", object, "-"}, probe.String(), maxDiagnostics); err != nil {
					t.Fatal(triple, err)
				}
				data, err := os.ReadFile(object)
				if err != nil {
					t.Fatal(err)
				}
				hasNamedBits := false
				for _, record := range h.Records {
					hasNamedBits = hasNamedBits || len(record.Bitfields) != 0
				}
				if !hasNamedBits {
					continue // Layout-only probes need no emitted mask symbols.
				}
				symbols, err := readBitfieldSymbols(data, h.Target.OS)
				if err != nil {
					t.Fatal(err)
				}
				for index, r := range h.Records {
					for _, f := range r.Bitfields {
						value, err := symbols.value(fmt.Sprintf("dylib_bitfield_%d_%d", index, f.Position), r.Layout.Size)
						want := make([]byte, r.Layout.Size)
						copy(want[r.Layout.Offsets[f.Position]:], f.Mask)
						if err != nil || !bytes.Equal(value, want) {
							t.Fatal(triple, r.Name, f.Position, value, want, err)
						}
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
			t.Parallel()
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
			for _, marker := range []string{"Bitfields:", "uint32_t f0:3;", "int32_t f1:5;", "_Bool f2:1;", "_mask(0,UINT64_MAX)", "_from_transport("} {
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
