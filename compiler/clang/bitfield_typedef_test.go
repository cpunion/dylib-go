package clang

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

var bitfieldTypedefFunctions = []string{"echo_low", "echo_high", "echo_low_wide", "echo_high_wide", "echo_down", "echo_shared", "echo_float", "echo_packed", "echo_pragma", "echo_maximum", "echo_choice", "echo_envelope", "echo_member", "echo_member_packed", "echo_boundary", "echo_padding", "echo_packed_parent"}

func bitfieldTypedefFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../examples/typedefbitfields/testdata/exports.h")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestBitfieldTypedefLayouts(t *testing.T) {
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
		if target.os == "windows" {
			producers = append(producers, strings.Replace(target.triple, "pc-windows-msvc", "w64-windows-gnu", 1))
		}
	}
	testCompilerBridgeTargets(t, typedefCompilerTool(t), bitfieldTypedefFixture(t), nil, producers, bitfieldTypedefFunctions...)
}

func TestBitfieldTypedefMetadata(t *testing.T) {
	path, compiler := headerFile(t, bitfieldTypedefFixture(t)), typedefCompilerTool(t)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: bitfieldTypedefFunctions})
			if errors.Is(err, errTypedefABI) {
				t.Log(err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if target.os != "windows" || target.arch != "386" {
				testCompilerRecordCallingABI(t, compiler, path, h)
			}
			low, _ := h.LookupRecord("LowTiny")
			high, _ := h.LookupRecord("HighTiny")
			wide, _ := h.LookupRecord("HighWide")
			if low.Bitfields[0].TypeAlignment != 1 || high.Bitfields[0].TypeAlignment != 16 || len(low.FieldTypeAlignments) != 0 {
				t.Fatal("typedef request lost or conflated with ordinary member alignment", low, high)
			}
			if target.os != "windows" && (low.Layout.Size != 3 || high.Layout.Size != 16 || high.Layout.Offsets[1] != 1 || wide.Layout.Offsets[1] != 16) {
				t.Fatal("short/wide typedef unit placement", low, high, wide)
			}
			wantArgumentAlignment := uint64(0)
			if target.os == "linux" && target.arch == "arm64" {
				wantArgumentAlignment = 16
			}
			if high.ArgumentAlignment != wantArgumentAlignment {
				t.Fatal("natural argument alignment", high.ArgumentAlignment, wantArgumentAlignment)
			}
			if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
				t.Fatal("typedef bitfields accepted without a C bridge")
			}
			high.Bitfields[0].TypeAlignment = 32
			fresh, _ := h.LookupRecord("HighTiny")
			if fresh.Bitfields[0].TypeAlignment != 16 {
				t.Fatal("shared bitfield metadata")
			}
		})
	}
}

func TestBitfieldTypedefGCCBridge(t *testing.T) {
	testCompilerRecordGCCBridge(t, typedefCompilerTool(t), bitfieldTypedefFixture(t), bitfieldTypedefFunctions)
}

func TestBitfieldTypedefDefaultPacking(t *testing.T) {
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	testCompilerBridgeTargets(t, typedefCompilerTool(t), bitfieldTypedefFixture(t), []string{"-fpack-struct=1"}, producers, bitfieldTypedefFunctions...)
}

func TestBitfieldTypedefValidation(t *testing.T) {
	base := Record{Name: "S", Description: abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "bits", Type: abi.TypeDesc{Type: abi.U32}}}}, Layout: abi.Layout{Size: 16, Alignment: 16, Offsets: []uint64{0}}, Bitfields: []Bitfield{{Position: 0, Width: 3, Mask: []byte{7}, TypeAlignment: 16, StorageAlignment: 16}}}
	if err := validRecordLayout(base); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]uint64{{3, 0}, {131072, 0}, {0, 1}, {16, 3}, {16, 32}, {16, 131072}} {
		r := base
		r.Bitfields = cloneBitfields(base.Bitfields)
		r.Bitfields[0].TypeAlignment, r.Bitfields[0].StorageAlignment = pair[0], pair[1]
		if err := validRecordLayout(r); err == nil {
			t.Fatal("invalid typedef boundary accepted", pair)
		}
	}
	shifted := base
	shifted.Bitfields = cloneBitfields(base.Bitfields)
	shifted.Bitfields[0].Mask = []byte{14}
	if err := validRecordLayout(shifted); err == nil {
		t.Fatal("storage boundary inside a byte accepted")
	}
	other := base
	other.Bitfields = cloneBitfields(base.Bitfields)
	other.Bitfields[0].TypeAlignment = 32
	if _, err := newDirectTypes([]Record{base, other}, "Bindings"); err == nil {
		t.Fatal("distinct bitfield typedefs collapsed")
	}
	base.ArgumentAlignment = 16
	linuxARM64, err := parseTarget("aarch64-unknown-linux-gnu", 8)
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Header{Target: linuxARM64, Records: []Record{base}}).validateDescriptions(); err != nil {
		t.Fatal(err)
	}
	for _, triple := range []string{"arm64-apple-macosx11", "x86_64-unknown-linux-gnu", "aarch64-pc-windows-msvc"} {
		target, err := parseTarget(triple, 8)
		if err != nil {
			t.Fatal(err)
		}
		if err := (&Header{Target: target, Records: []Record{base}}).validateDescriptions(); err == nil {
			t.Fatal("argument marker accepted on another ABI", triple)
		}
	}
	for _, layout := range []abi.Layout{{Size: 16, Alignment: 8, Offsets: []uint64{0}}, {Size: 32, Alignment: 16, Offsets: []uint64{0}}} {
		r := base
		r.Layout = layout
		if err := validRecordLayout(r); err == nil {
			t.Fatal("argument marker accepted on incompatible storage", layout)
		}
	}
	other = base
	other.ArgumentAlignment = 0
	if _, err := newDirectTypes([]Record{base, other}, "Bindings"); err == nil {
		t.Fatal("distinct natural argument alignments collapsed")
	}
}

func TestBitfieldTypedefNaturalArgumentAlignment(t *testing.T) {
	source := `typedef unsigned Low __attribute__((aligned(1)));
typedef struct __attribute__((aligned(16))) { Low bits:3; } Explicit;
Explicit f(Explicit);`
	path, compiler := headerFile(t, source), typedefCompilerTool(t)
	h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: "aarch64-unknown-linux-gnu", Functions: []string{"f"}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := h.LookupRecord("Explicit")
	if err != nil || r.ArgumentAlignment != 0 {
		t.Fatal("explicit alignment was treated as natural argument alignment", r, err)
	}
	testCompilerRecordCallingABI(t, compiler, path, h)
}

func TestBitfieldTypedefPadding(t *testing.T) {
	source := `typedef unsigned High __attribute__((aligned(16)));
typedef unsigned Low __attribute__((aligned(1)));
typedef struct { char tag; High bits:3; unsigned :0; char tail; } Boundary;
typedef struct { Low bits:3; unsigned :5; High next:31; char tail; } SharedPadding;
typedef struct { char aligned_tag; High aligned_bits:3; unsigned :0 __attribute__((aligned(8))); char aligned_tail; } AlignedBoundary;
Boundary f(Boundary); SharedPadding g(SharedPadding); AlignedBoundary h(AlignedBoundary);`
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	testCompilerBridgeTargets(t, typedefCompilerTool(t), source, nil, producers, "f", "g", "h")
}

func TestBitfieldTypedefPackedParent(t *testing.T) {
	source := `typedef unsigned High __attribute__((aligned(16)));
typedef struct __attribute__((packed)) { char tag; High bits:31; short tail; } Packed;
#pragma pack(push,1)
typedef struct { char parent_tag; Packed child; char parent_tail; } Parent;
#pragma pack(pop)
Packed f(Packed); Parent g(Parent);`
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	testCompilerBridgeTargets(t, typedefCompilerTool(t), source, nil, producers, "f", "g")
}
