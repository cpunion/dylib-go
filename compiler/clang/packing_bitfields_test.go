package clang

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func pragmaBitfieldHeader() (string, []string) {
	bodies := []struct{ name, fields, attributes string }{
		{"Mixed", "signed char tag; unsigned low:3; signed high:5; _Bool active:1; unsigned long long wide:40; double real;", ""},
		{"Tiny", "unsigned low:3; signed high:5;", ""},
		{"Gaps", "unsigned low:3; unsigned :0; signed high:7; double real;", ""},
		{"Leading", "unsigned long long :0; unsigned :0; float left; float right;", ""},
		{"Boundaries", "signed char tag; unsigned low:3; unsigned :4; unsigned long long :0; unsigned :0; signed high:5; unsigned :0; signed char tail;", ""},
		{"PaddingOnly", "signed char tag; unsigned :3; signed char tail;", ""},
		{"PackedMember", "signed char tag; unsigned low:3 __attribute__((packed)); signed high:5; double real;", ""},
		{"PackedZero", "signed char tag; unsigned low:3; unsigned long long :0 __attribute__((packed)); signed char tail;", ""},
		{"Aligned", "signed char tag; unsigned low:3 __attribute__((aligned(1))); signed high:5; double real;", ""},
		{"PaddingAligned", "signed char tag; unsigned :3 __attribute__((aligned(1))); unsigned low:5; unsigned :0; double real;", ""},
		{"Global", "signed char tag; unsigned :3; unsigned low:5; unsigned long long :0; signed high:7; unsigned :0; double real;", "__attribute__((packed))"},
		{"GlobalAligned", "signed char tag; unsigned :3 __attribute__((aligned(1))); unsigned low:5; unsigned long long :0; double real;", "__attribute__((packed,aligned(32)))"},
	}
	var source strings.Builder
	var names []string
	fields := regexp.MustCompile(`\b(tag|low|high|active|wide|real|left|right|tail)\b`)
	for _, cap := range []int{1, 2, 4, 8, 16} {
		fmt.Fprintf(&source, "#pragma pack(push,%d)\n", cap)
		for _, body := range bodies {
			name := fmt.Sprintf("%s%d", body.name, cap)
			declarations := fields.ReplaceAllStringFunc(body.fields, func(field string) string { return field + name })
			fmt.Fprintf(&source, "typedef struct %s { %s } %s;\n%s echo_%s(%s);\n", body.attributes, declarations, name, name, name, name)
			names = append(names, "echo_"+name)
		}
		source.WriteString("#pragma pack(pop)\n")
	}
	return source.String(), names
}

func TestPragmaBitfieldsAcrossTargets(t *testing.T) {
	source, names := pragmaBitfieldHeader()
	path, compiler := headerFile(t, source), compilerTool(t)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: names})
			if err != nil {
				t.Fatal(err)
			}
			if len(h.Records) != len(names) {
				t.Fatal("missing pragma bitfield records", len(h.Records))
			}
			for _, cap := range []uint64{1, 2, 4, 8, 16} {
				r, err := h.LookupRecord(fmt.Sprintf("Boundaries%d", cap))
				if err != nil || r.MaxFieldAlignment != cap || len(r.Padding) != 4 || len(r.Bitfields) != 2 {
					t.Fatal("pragma boundaries", r, err)
				}
			}
			if _, err := h.CgoSource("bindings", "Bindings"); err != nil {
				t.Fatal(err)
			}
			if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
				t.Fatal("pragma bitfields accepted by direct llgo adapter")
			}
		})
	}
}

func TestPragmaBitfieldBridgeMSVCAndMinGW(t *testing.T) {
	source, names := pragmaBitfieldHeader()
	testPackedBitfieldBridgeMSVCAndMinGW(t, source, names...)
}

func TestPragmaBitfieldNativeFixtureBridges(t *testing.T) {
	path, err := filepath.Abs("../../examples/bitfieldrecords/testdata/exports.h")
	if err != nil {
		t.Fatal(err)
	}
	testPackedBitfieldBridgeMSVCAndMinGW(t, fmt.Sprintf("#include %q\n", filepath.ToSlash(path)),
		"echo_pragma_bits", "echo_pragma_tiny", "echo_pragma_packed", "echo_pragma_float", "echo_pragma_bits_envelope", "apply_pragma_bits_slot")
}
