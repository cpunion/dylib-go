package clang

import (
	"context"
	"os"
	"strings"
	"testing"
)

var plainZeroFunctions = []string{"echo_parent", "echo_float", "echo_gap", "echo_bits", "echo_trailing", "echo_leading", "echo_consecutive", "echo_nonpower", "echo_large", "echo_mixed", "echo_packed", "echo_envelope", "shifted_float", "gap_overflow", "gap_variadic", "float_factory", "forward_float", "gap_pointer", "read_gap"}

func plainZeroFixture(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("../../examples/zeroboundaries/testdata/exports.h")
	if err != nil {
		t.Fatal(err)
	}
	return string(source)
}

func TestPlainZeroBoundaryBridge(t *testing.T) {
	compiler := typedefCompilerTool(t)
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	for _, flags := range [][]string{nil, {"-fpack-struct=1"}} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			testCompilerBridgeTargetsWithInspection(t, compiler, plainZeroFixture(t), flags, producers, func(t *testing.T, path string, h *Header) {
				if len(h.Records) != 13 {
					t.Fatal("missing zero-width records", len(h.Records))
				}
				if len(flags) == 0 && !(h.Target.OS == "windows" && h.Target.Arch == "386") {
					testCompilerRecordCallingABI(t, compiler, path, h)
				}
				if h.Target.gnuPacking() {
					parent, err := h.LookupRecord("Parent")
					alignment, size := uint64(2), uint64(10)
					if h.Target.OS == "linux" && h.Target.Arch == "arm64" {
						alignment, size = 4, 12
					}
					if err != nil || parent.Layout.Size != size || parent.Layout.Alignment != alignment || parent.Padding[0].Gap != 2 {
						t.Fatal("plain boundary observation", parent, err)
					}
					trailing, _ := h.LookupRecord("Trailing")
					trailingAlignment := uint64(1)
					if h.Target.OS == "linux" && h.Target.Arch == "arm64" {
						trailingAlignment = 4
					}
					if trailing.Layout.Size != 4 || trailing.Layout.Alignment != trailingAlignment || trailing.Padding[0].Gap != 3 {
						t.Fatal("trailing boundary", trailing)
					}
					nonpower, _ := h.LookupRecord("NonPower")
					if nonpower.Padding[0].Offset != 12 || nonpower.Padding[0].Gap != 2 {
						t.Fatal("non-power-of-two boundary", nonpower)
					}
				}
			}, plainZeroFunctions...)
		})
	}
}

func TestPlainZeroBoundaryGCCBridge(t *testing.T) {
	testCompilerRecordGCCBridge(t, typedefCompilerTool(t), plainZeroFixture(t), plainZeroFunctions)
}

func TestPlainZeroBoundaryLegacyPragma(t *testing.T) {
	source, names := pragmaBitfieldHeader()
	var zeroNames []string
	for _, name := range names {
		// These have no zero width. GNU unnamed positive-width root alignment
		// is a separate remaining qualification item.
		if !strings.Contains(name, "PaddingOnly") {
			zeroNames = append(zeroNames, name)
		}
	}
	compiler := typedefCompilerTool(t)
	var producers []string
	for _, target := range targets {
		if target.os != "windows" {
			producers = append(producers, target.triple)
		}
	}
	testCompilerBridgeTargetsWithInspection(t, compiler, source, nil, producers, func(t *testing.T, path string, h *Header) {
		if len(h.Records) != len(zeroNames) {
			t.Fatal("missing legacy pragma records", len(h.Records))
		}
		testCompilerRecordCallingABI(t, compiler, path, h)
	}, zeroNames...)
	testCompilerRecordGCCBridge(t, compiler, source, zeroNames)
}

func TestPlainZeroBoundaryUnsupportedSysV(t *testing.T) {
	source := `#pragma pack(push,2)
 typedef struct {float first; unsigned :3; unsigned :0; float tail;} Mixed;
 #pragma pack(pop)
 Mixed f(Mixed);`
	h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: typedefCompilerTool(t), Target: "x86_64-unknown-linux-gnu", Functions: []string{"f"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.CgoSource("bindings", "Bindings"); err == nil || !strings.Contains(err.Error(), "implicit-padding C bridge") {
		t.Fatal("unqualified original unnamed value accepted", err)
	}
}
