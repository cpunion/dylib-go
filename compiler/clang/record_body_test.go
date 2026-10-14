package clang

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

var anonymousRecordFunctions = []string{"echo_high", "echo_inherited", "echo_low", "echo_low_only", "echo_float", "echo_wide", "echo_wide_low", "echo_doubles", "echo_maximum", "echo_packed", "echo_pragma", "echo_member", "echo_bits", "echo_nested", "echo_array", "echo_explicit", "shifted", "overflow", "wide_overflow", "variadic", "factory", "forward", "pointer", "read_high", "alignment"}

func anonymousRecordFixture(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("../../examples/anonymousrecords/testdata/exports.h")
	if err != nil {
		t.Fatal(err)
	}
	return string(source)
}

func TestAnonymousRecordTypedefBridge(t *testing.T) {
	source, compiler := anonymousRecordFixture(t), typedefCompilerTool(t)
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
		if target.os == "windows" {
			producers = append(producers, strings.Replace(target.triple, "pc-windows-msvc", "w64-windows-gnu", 1))
		}
	}
	for _, flags := range [][]string{nil, {"-fpack-struct=1"}} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			testCompilerBridgeTargetsWithInspection(t, compiler, source, flags, producers, func(t *testing.T, path string, h *Header) {
				r, err := h.LookupRecord("High")
				alignment := uint64(4)
				if len(flags) != 0 {
					alignment = 1
				}
				if err != nil || len(h.Records) != 14 || !reflect.DeepEqual(r.Layout, abi.Layout{Size: 4, Alignment: alignment, Offsets: []uint64{0}}) || !reflect.DeepEqual(r.TypedefAlignments, []RecordTypedefAlignment{{Name: "High", Aligned: 16}, {Name: "Inherited", Aligned: 16}, {Name: "Low", Aligned: 1}}) {
					t.Fatal("anonymous alias/body alignment conflated", r, err)
				}
				if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
					t.Fatal("anonymous aliases accepted without a C bridge")
				}
				if len(flags) == 0 && !(h.Target.OS == "windows" && h.Target.Arch == "386") && !strings.Contains(h.Target.Triple, "windows-gnu") {
					testCompilerRecordCallingABI(t, compiler, path, h)
				}
			}, anonymousRecordFunctions...)
		})
	}
}

func TestAnonymousRecordTypedefGCCBridge(t *testing.T) {
	testCompilerRecordGCCBridge(t, typedefCompilerTool(t), anonymousRecordFixture(t), anonymousRecordFunctions)
}

func TestAnonymousRecordTypedefInheritedOnly(t *testing.T) {
	source := `typedef struct {int value;} High __attribute__((aligned(16))); typedef High Alias; Alias f(Alias);`
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: typedefCompilerTool(t), Target: target.triple, Functions: []string{"f"}})
			if err != nil || len(h.Records) != 1 {
				t.Fatal(h, err)
			}
			r := h.Records[0]
			if r.Name != "High" || r.Layout.Alignment != 4 || r.Layout.Size != 4 || !reflect.DeepEqual(r.TypedefAlignments, []RecordTypedefAlignment{{Name: "Alias", Aligned: 16}, {Name: "High", Aligned: 16}}) {
				t.Fatal("inherited-only alias changed the body", r)
			}
		})
	}
}

func TestAnonymousRecordTypedefUnclosedPacking(t *testing.T) {
	source := `typedef struct { int value; } High __attribute__((aligned(16)));
#pragma pack(push,1)
High f(High);`
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: typedefCompilerTool(t), Target: target.triple, Functions: []string{"f"}})
			if err != nil || h.Records[0].Layout.Alignment != 4 {
				t.Fatal("header's trailing pragma changed a synthetic body", h, err)
			}
		})
	}
}

func TestAnonymousRecordTypedefCallbackOnly(t *testing.T) {
	source := `typedef struct {int value;} I __attribute__((aligned(16))); int f(I (*callback)(I));`
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: typedefCompilerTool(t), Target: target.triple, Functions: []string{"f"}})
			if err != nil || len(h.Records) != 1 || len(h.Functions[0].FunctionPointers) != 1 {
				t.Fatal(h, err)
			}
			pointer := h.Functions[0].FunctionPointers[0]
			if pointer.Signature.Args[0] != abi.Struct || pointer.Signature.Result != abi.Struct || h.Records[0].Layout.Alignment != 4 {
				t.Fatal("callback-only anonymous body lost", h)
			}
			if _, err := h.CgoSource("bindings", "Bindings"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
