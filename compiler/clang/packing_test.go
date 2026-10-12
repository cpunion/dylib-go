package clang

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func pragmaPackingHeader() (string, []string) {
	var source strings.Builder
	var functions []string
	for _, cap := range []int{1, 2, 4, 8, 16} {
		fmt.Fprintf(&source, "#pragma pack(push,%d)\ntypedef struct { char tag%d; double value%d; int tail%d; } Pack%d;\n#pragma pack(pop)\nPack%d pack%d(Pack%d);\n", cap, cap, cap, cap, cap, cap, cap, cap)
		functions = append(functions, fmt.Sprintf("pack%d", cap))
	}
	source.WriteString(`#pragma pack(push,2)
typedef struct __attribute__((aligned(32))) { char aligned_tag; int aligned_value; double aligned_tail; } Aligned;
#pragma pack(push,4)
typedef struct { char envelope_tag; Pack2 items[2][2]; int count; } Envelope;
#pragma pack(pop)
typedef struct { char restored_tag; double restored_value; } Restored;
#pragma pack(pop)
typedef struct { char natural_tag; double natural_value; } Natural;
#pragma pack(push,2)
typedef struct { char callback_tag; int (*entry)(int); } CallbackSlot;
#pragma pack(pop)
Aligned aligned(Aligned); Envelope envelope(Envelope); Restored restored(Restored); Natural natural(Natural); CallbackSlot slot(CallbackSlot);
`)
	return source.String(), append(functions, "aligned", "envelope", "restored", "natural", "slot")
}

func TestPragmaPackingAcrossTargets(t *testing.T) {
	source, functions := pragmaPackingHeader()
	path, compiler := headerFile(t, source), compilerTool(t)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: functions})
			if err != nil {
				t.Fatal(err)
			}
			for _, cap := range []uint64{1, 2, 4, 8, 16} {
				r, err := h.LookupRecord(fmt.Sprintf("Pack%d", cap))
				if err != nil || r.MaxFieldAlignment != cap || r.Packed {
					t.Fatal("packing request:", r, err)
				}
			}
			aligned, _ := h.LookupRecord("Aligned")
			envelope, _ := h.LookupRecord("Envelope")
			restored, _ := h.LookupRecord("Restored")
			natural, _ := h.LookupRecord("Natural")
			if aligned.MaxFieldAlignment != 2 || aligned.Layout.Alignment != 32 || aligned.Layout.Offsets[1] != 2 || envelope.MaxFieldAlignment != 4 || restored.MaxFieldAlignment != 2 || natural.MaxFieldAlignment != 0 {
				t.Fatal("packing stack and record alignment:", aligned, envelope, restored, natural)
			}
			slot, _ := h.LookupRecord("CallbackSlot")
			entry, err := slot.LookupFunctionPointer("entry")
			if err != nil || slot.MaxFieldAlignment != 2 || entry.Signature.Result != abi.I32 || len(entry.Signature.Args) != 1 || entry.Signature.Args[0] != abi.I32 {
				t.Fatal("packed function-pointer field:", slot, entry, err)
			}
			generated, err := h.GoSource("bindings", "Declarations")
			if err != nil || !strings.Contains(string(generated), "MaxFieldAlignment: 2") {
				t.Fatal("packing metadata persistence:", err)
			}
			if _, err := h.CgoSource("bindings", "Bindings"); err != nil {
				t.Fatal(err)
			}
			if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
				t.Fatal("pragma packing accepted by direct llgo adapter")
			}
		})
	}
}

func TestPragmaPackingBridgeMSVCAndMinGW(t *testing.T) {
	source, functions := pragmaPackingHeader()
	testPackedBitfieldBridgeMSVCAndMinGW(t, source, functions...)
}

func TestPragmaPackingRequiresCAdapter(t *testing.T) {
	h, err := Parse(context.Background(), headerFile(t, "#pragma pack(push,2)\ntypedef struct { int value; } S;\n#pragma pack(pop)\nS echo(S);"), Options{Compiler: compilerTool(t), Functions: []string{"echo"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.ForHost("echo"); err == nil || !strings.Contains(err.Error(), "requires a compiled C adapter") {
		t.Fatal("pragma record accepted by dynamic backend:", err)
	}
}

func TestDecodePackingProbes(t *testing.T) {
	const dump = "TranslationUnitDecl 0x0\n|-RecordDecl 0x1 struct definition\n| `-MaxFieldAlignmentAttr 0x2 Implicit 16\n`-TypedefDecl 0x3 dylib_record_packing_0 'S'\n  `-RecordType 0x4 'S'\n    `-Record 0x1 'S'\n"
	keys := map[string]string{"dylib_record_packing_0": "S"}
	got, err := decodePackingProbes([]byte(dump), keys)
	if err != nil || got["S"] != 2 {
		t.Fatal(got, err)
	}
	for _, malformed := range []string{
		"", strings.Replace(dump, "Implicit 16", "Implicit 0", 1),
		strings.Replace(dump, "Implicit 16", "Implicit 24", 1),
		strings.Replace(dump, "Implicit 16", "Implicit 256", 1),
		strings.Replace(dump, "Implicit 16", "Implicit unknown", 1),
		strings.Replace(dump, "Record 0x1", "Record 0x9", 1),
		dump + "|-TypedefDecl 0x5 dylib_record_packing_0 'S'\n",
		strings.Replace(dump, "| `-MaxFieldAlignmentAttr 0x2 Implicit 16", "| |-MaxFieldAlignmentAttr 0x2 Implicit 16\n| `-MaxFieldAlignmentAttr 0x5 Implicit 16", 1),
		dump + "    `-Record 0x1 'S'\n",
	} {
		if _, err := decodePackingProbes([]byte(malformed), keys); err == nil {
			t.Fatal("invalid packing dump accepted:", malformed)
		}
	}
}

func TestInvalidPackingMetadata(t *testing.T) {
	for _, cap := range []uint64{3, 32, 65536} {
		if err := validatePacking(Record{MaxFieldAlignment: cap}); err == nil {
			t.Fatal("invalid packing cap accepted", cap)
		}
	}
	if err := validatePacking(Record{MaxFieldAlignment: 2, Padding: []BitfieldPadding{{Type: abi.U32, Width: 0, Aligned: 16}}}); err == nil {
		t.Fatal("unqualified over-aligned unnamed member accepted")
	}
	if err := validatePacking(Record{MaxFieldAlignment: 2, FieldAlignments: []FieldAlignment{{Aligned: 16}}}); err == nil {
		t.Fatal("unqualified over-aligned pragma member accepted")
	}
	if _, _, err := recordPackingProbes([]byte("broken")); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if _, _, err := recordPackingProbes([]byte(`{"kind":"TranslationUnitDecl"}`)); err == nil {
		t.Fatal("absent packing declaration accepted")
	}
}

func TestPackingABIQualification(t *testing.T) {
	for _, source := range []string{
		"#pragma pack(push,2)\ntypedef struct { unsigned bits:3 __attribute__((aligned(16))); } S;\n#pragma pack(pop)\nS echo(S);",
		"#pragma pack(push,2)\ntypedef struct { char tag; int value __attribute__((aligned(16))); } S;\n#pragma pack(pop)\nS echo(S);",
		"typedef struct __attribute__((aligned(32))) { int value; } Aligned;\n#pragma pack(push,2)\ntypedef struct { char tag; Aligned items[2]; } S;\n#pragma pack(pop)\nS echo(S);",
		"typedef struct { unsigned :3 __attribute__((aligned(16))); unsigned value:5; } Bits;\n#pragma pack(push,2)\ntypedef struct { char tag; Bits items[2]; } S;\n#pragma pack(pop)\nS echo(S);",
	} {
		if _, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: compilerTool(t), Functions: []string{"echo"}}); err == nil {
			t.Fatal("unqualified packing combination accepted:", source)
		}
	}
	description := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "value", Type: abi.TypeDesc{Type: abi.I32}}}}
	records := []Record{
		{Name: "Pack4", Description: description, MaxFieldAlignment: 4, Layout: abi.Layout{Size: 4, Alignment: 4, Offsets: []uint64{0}}},
		{Name: "Pack8", Description: description, MaxFieldAlignment: 8, Layout: abi.Layout{Size: 4, Alignment: 4, Offsets: []uint64{0}}},
	}
	if _, err := newDirectTypes(records, "Bindings"); err == nil {
		t.Fatal("distinct packing requests collapsed despite equal logical/native layouts")
	}
}
