package clang

import (
	"context"
	"errors"
	"github.com/cpunion/dylib-go/abi"
	"os"
	"strings"
	"testing"
)

var paddingTypedefFunctions = []string{"echo_low", "echo_high", "echo_low_wide", "echo_high_wide", "echo_zero", "echo_active", "echo_zero_low", "echo_shared", "echo_float", "echo_packed", "echo_pragma", "echo_maximum", "echo_choice", "echo_envelope", "echo_member", "echo_small", "echo_tail", "echo_down", "echo_consecutive", "echo_parent", "echo_maximum_zero", "echo_leading", "echo_aligned"}

func paddingTypedefFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../examples/paddingtypedefs/testdata/exports.h")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPaddingTypedefLayouts(t *testing.T) {
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
		if target.os == "windows" {
			producers = append(producers, strings.Replace(target.triple, "pc-windows-msvc", "w64-windows-gnu", 1))
		}
	}
	compiler := typedefCompilerTool(t)
	inspect := func(t *testing.T, path string, h *Header) {
		// Retain the original eight-target calling checks; Windows GNU producers
		// additionally qualify layout/bridge compatibility below.
		if h.Target.OS == "windows" && !h.Target.msLayout() {
			return
		}
		if h.Target.OS != "windows" || h.Target.Arch != "386" {
			testCompilerRecordCallingABI(t, compiler, path, h)
		}
		low, _ := h.LookupRecord("LowTiny")
		high, _ := h.LookupRecord("HighTiny")
		wide, _ := h.LookupRecord("HighWide")
		if low.Padding[0].TypeAlignment != 1 || high.Padding[0].TypeAlignment != 16 {
			t.Fatal("missing base typedef request", low, high)
		}
		if h.Target.OS != "windows" && (low.Layout.Size != 3 || high.Layout.Offsets[1] != 2 || wide.Layout.Offsets[1] != 20) {
			t.Fatal("typedef allocation placement", low, high, wide)
		}
		high.Padding[0].TypeAlignment = 32
		fresh, _ := h.LookupRecord("HighTiny")
		if fresh.Padding[0].TypeAlignment != 16 {
			t.Fatal("shared metadata")
		}
	}
	testCompilerBridgeTargetsWithInspection(t, compiler, paddingTypedefFixture(t), nil, producers, inspect, paddingTypedefFunctions...)
}

func TestPaddingTypedefGCCBridge(t *testing.T) {
	testCompilerRecordGCCBridge(t, typedefCompilerTool(t), paddingTypedefFixture(t), paddingTypedefFunctions)
}
func TestPaddingTypedefDefaultPacking(t *testing.T) {
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	names := append(append([]string(nil), paddingTypedefFunctions[:16]...), paddingTypedefFunctions[17:]...)
	testCompilerBridgeTargets(t, typedefCompilerTool(t), paddingTypedefFixture(t), []string{"-fpack-struct=1"}, producers, names...)
}

func TestPaddingTypedefMemberAttributes(t *testing.T) {
	source := `typedef unsigned Low __attribute__((aligned(1)));
 typedef unsigned High __attribute__((aligned(16)));
 typedef struct { char tag; High :3 __attribute__((aligned(8))); char tail; } Member;
 typedef struct { char packed_tag; High :31 __attribute__((packed,aligned(8))); short packed_tail; } PackedMember;
 typedef struct __attribute__((packed)) { char zero_tag; Low bits:3; High :0; char zero_tail; } PackedZero;
 Member f(Member); PackedMember g(PackedMember); PackedZero h(PackedZero);`
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	testCompilerBridgeTargets(t, typedefCompilerTool(t), source, nil, producers, "f", "g", "h")
	testCompilerRecordGCCBridge(t, typedefCompilerTool(t), source, []string{"f", "g", "h"})
}

func TestPaddingTypedefValidation(t *testing.T) {
	base := Record{Name: "S", Description: abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "tag", Type: abi.TypeDesc{Type: abi.I8}}, {Name: "tail", Type: abi.TypeDesc{Type: abi.I8}}}}, Layout: abi.Layout{Size: 21, Alignment: 1, Offsets: []uint64{0, 20}}, Padding: []BitfieldPadding{{Before: 1, Type: abi.U32, Width: 31, TypeAlignment: 16, StorageAlignment: 16, Offset: 16, Gap: 15}}}
	if err := validRecordLayout(base); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*BitfieldPadding){
		func(p *BitfieldPadding) { p.TypeAlignment = 3 }, func(p *BitfieldPadding) { p.TypeAlignment = 131072 },
		func(p *BitfieldPadding) { p.TypeAlignment = 0 }, func(p *BitfieldPadding) { p.StorageAlignment = 3 },
		func(p *BitfieldPadding) { p.Offset = 17 }, func(p *BitfieldPadding) { p.Offset = 21 },
		func(p *BitfieldPadding) { p.Gap = 17 }, func(p *BitfieldPadding) { p.Gap = 65537 },
	} {
		r := base
		r.Padding = append([]BitfieldPadding(nil), base.Padding...)
		change(&r.Padding[0])
		if err := validRecordLayout(r); err == nil {
			t.Fatal("invalid unnamed typedef metadata", r)
		}
	}
	other := base
	other.Padding = append([]BitfieldPadding(nil), base.Padding...)
	other.Padding[0].TypeAlignment = 32
	if _, err := newDirectTypes([]Record{base, other}, "Bindings"); err == nil {
		t.Fatal("distinct base typedefs collapsed")
	}
}

func TestPaddingTypedefUnsupportedSysVGap(t *testing.T) {
	for _, source := range []string{
		`typedef unsigned Eight __attribute__((aligned(8))); typedef struct {float x; Eight :31; char tail;} S; S f(S);`,
		`typedef unsigned Eight __attribute__((aligned(8))); typedef struct {float x; unsigned :3; Eight :0; char tail;} S; S f(S);`,
	} {
		h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: typedefCompilerTool(t), Target: "x86_64-unknown-linux-gnu", Functions: []string{"f"}})
		if errors.Is(err, errTypedefABI) {
			t.Log(err)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err = h.CgoSource("bindings", "Bindings"); err == nil || !strings.Contains(err.Error(), "implicit-padding C bridge") {
			t.Fatal("unqualified SysV gap accepted", err)
		}
	}
}
