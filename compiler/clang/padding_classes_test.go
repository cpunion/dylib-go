package clang

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// These expected outcomes also have independent Clang-producer/GCC-caller
// reproductions: 11 mismatches and 16 working controls on Linux AMD64.
func TestUnnamedPaddingRegisterClasses(t *testing.T) {
	var source strings.Builder
	type testCase struct {
		name   string
		reject bool
	}
	var cases []testCase
	for _, mode := range []string{"float_int", "int_float", "float_int_float", "integer", "wide"} {
		for _, width := range []int{1, 3, 31, 32, 33, 40, 64} {
			if mode != "integer" && width == 64 || mode == "wide" && width < 33 {
				continue
			}
			name := fmt.Sprintf("V_%s_%d", mode, width)
			base := "unsigned"
			if width > 32 {
				base = "unsigned long long"
			}
			fields := fmt.Sprintf("float first; %s :%d; int last;", base, width)
			reject := width <= 32
			switch mode {
			case "int_float":
				fields, reject = fmt.Sprintf("int first; %s :%d; float last;", base, width), false
			case "float_int_float":
				fields = fmt.Sprintf("float first; int guard; %s :%d; float last;", base, width)
			case "integer":
				fields, reject = fmt.Sprintf("%s :%d; int last;", base, width), width > 32
			case "wide":
				fields, reject = fmt.Sprintf("float first; %s :%d; signed char last;", base, width), false
			}
			fmt.Fprintf(&source, "typedef struct {%s} %s; %s echo_%s(%s);\n", fields, name, name, name, name)
			cases = append(cases, testCase{name, reject})
		}
	}
	path, compiler := headerFile(t, source.String()), compilerTool(t)
	for _, tc := range cases {
		for _, triple := range []string{"x86_64-unknown-linux-gnu", "x86_64-apple-macosx11"} {
			t.Run(tc.name+"/"+triple, func(t *testing.T) {
				t.Parallel()
				h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: triple, Functions: []string{"echo_" + tc.name}})
				if err != nil {
					t.Fatal(err)
				}
				_, err = h.CgoSource("bindings", "Bindings")
				if (err != nil) != tc.reject || err != nil && !strings.Contains(err.Error(), "implicit-padding C bridge") {
					t.Fatal("wrong unnamed-field ABI qualification", err)
				}
			})
		}
	}
}

func TestUnnamedPaddingSafeBridges(t *testing.T) {
	source, err := os.ReadFile("../../examples/overalignedbits/testdata/exports.h")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"echo_class_wide", "echo_class_nested", "echo_class_array"}
	compiler := compilerTool(t)
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	testCompilerBridgeTargetsWithInspection(t, compiler, string(source), nil, producers, func(t *testing.T, path string, h *Header) {
		if len(h.Records) != 4 {
			t.Fatal("missing nested padding-class controls", len(h.Records))
		}
		if h.Target.OS != "windows" || h.Target.Arch != "386" {
			testCompilerRecordCallingABI(t, compiler, path, h)
		}
	}, names...)
	testCompilerRecordGCCBridge(t, compiler, string(source), names)
}

func TestUnnamedPaddingMaskMetadata(t *testing.T) {
	path := headerFile(t, "typedef struct {float first; unsigned long long :33; signed char last;} V; V echo(V);")
	h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: "x86_64-unknown-linux-gnu", Functions: []string{"echo"}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := h.LookupRecord("V")
	if err != nil || r.Padding[0].Offset != 8 || !reflect.DeepEqual(r.Padding[0].Mask, []byte{255, 255, 255, 255, 1}) {
		t.Fatal("missing compiler-observed padding", r, err)
	}
	r.Padding[0].Mask[0] = 0
	fresh, _ := h.LookupRecord("V")
	if fresh.Padding[0].Mask[0] != 255 {
		t.Fatal("padding mask snapshot is shared")
	}
	code, err := h.GoSource("bindings", "Declarations")
	if err != nil || !strings.Contains(string(code), "Mask: []byte{255, 255, 255, 255, 1}") {
		t.Fatal("padding observation missing from portable metadata", err)
	}
	for _, change := range []func(*BitfieldPadding){
		func(p *BitfieldPadding) { p.Width = 0 },
		func(p *BitfieldPadding) { p.Mask = []byte{5} },
		func(p *BitfieldPadding) { p.Mask = []byte{0, 255, 255, 255, 255, 1} },
		func(p *BitfieldPadding) { p.Mask = []byte{255} },
		func(p *BitfieldPadding) { p.Offset = 15 },
		func(p *BitfieldPadding) { p.Mask = nil },
	} {
		r, _ := h.LookupRecord("V")
		change(&r.Padding[0])
		bad := *h
		bad.Records = []Record{r}
		if _, err := bad.CgoSource("bindings", "Bindings"); err == nil {
			t.Fatal("invalid or unproved padding metadata accepted", r.Padding)
		}
	}
	missing := *h
	r, _ = h.LookupRecord("V")
	r.Padding[0].Mask, r.Padding[0].Offset = nil, 0
	missing.Records = []Record{r}
	if _, err := missing.CgoSource("bindings", "Bindings"); err == nil || !strings.Contains(err.Error(), "implicit-padding") {
		t.Fatal("missing observation used to infer floating/empty lanes", err)
	}
	legacy, err := Parse(context.Background(), headerFile(t, "typedef struct {unsigned :3; int value;} Legacy; Legacy echo(Legacy);"), Options{Compiler: compilerTool(t), Target: h.Target.Triple, Functions: []string{"echo"}})
	if err != nil {
		t.Fatal(err)
	}
	legacy.Records[0].Padding[0].Mask = nil
	if _, err := legacy.CgoSource("bindings", "Bindings"); err != nil {
		t.Fatal("fully integer legacy metadata rejected", err)
	}
}
