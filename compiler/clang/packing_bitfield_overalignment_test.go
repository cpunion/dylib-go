package clang

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func pragmaOverAlignedBitfields() (string, []string) {
	var source strings.Builder
	source.WriteString(`typedef struct __attribute__((aligned(32))) { unsigned inner_low:3; float inner_values[2]; } Inner;
Inner echo_inner(Inner);
`)
	functions := []string{"echo_inner"}
	patterns := []struct{ name, attributes, fields string }{
		{"Tiny", "", "unsigned low:3 __attribute__((aligned(8))); signed high:5 __attribute__((aligned(32)));"},
		{"Named", "", "char tag; unsigned low:3 __attribute__((aligned(16))); signed high:5 __attribute__((aligned(32))); _Bool truth:1 __attribute__((aligned(16))); unsigned long long wide:40 __attribute__((packed,aligned(32))); double real __attribute__((aligned(16))); void *context;"},
		{"Anonymous", "", "unsigned long long :0 __attribute__((aligned(32))); char tag; unsigned :3 __attribute__((aligned(16))); signed low:5; unsigned long long :0 __attribute__((aligned(32))); short high:7 __attribute__((aligned(16))); unsigned long long :0 __attribute__((aligned(16))); unsigned :0 __attribute__((aligned(64))); double real; void *context;"},
		{"Maximum", "", "char tag; unsigned low:3 __attribute__((aligned)); unsigned :0 __attribute__((aligned)); double real;"},
		{"Packed", "__attribute__((packed,aligned(32)))", "char tag; unsigned :3 __attribute__((aligned(16))); signed low:5; unsigned long long :0 __attribute__((aligned(32))); double real __attribute__((packed,aligned(16)));"},
		{"Nested", "", "char tag; Inner items[2]; unsigned low:3; double real;"},
		{"Float", "", "unsigned long long :0 __attribute__((aligned(32))); float left; float right; unsigned long long :0 __attribute__((aligned(16)));"},
		{"PaddingFloat", "", "unsigned :3 __attribute__((aligned(16))); float left; float right;"},
	}
	fields := regexp.MustCompile(`\b(tag|low|high|truth|wide|real|context|items|left|right)\b`)
	for _, cap := range []int{1, 2, 4, 8, 16} {
		for _, pattern := range patterns {
			if cap != 2 && pattern.name != "Named" && pattern.name != "Anonymous" && pattern.name != "Nested" {
				continue
			}
			name := fmt.Sprintf("%s%d", pattern.name, cap)
			members := fields.ReplaceAllString(pattern.fields, "${0}"+name)
			fmt.Fprintf(&source, "#pragma pack(push,%d)\ntypedef struct %s {%s} %s;\n#pragma pack(pop)\n%s echo_%s(%s);\n", cap, pattern.attributes, members, name, name, name, name)
			functions = append(functions, "echo_"+name)
		}
	}
	return source.String(), functions
}

func TestPragmaBitfieldOverAlignmentAcrossTargets(t *testing.T) {
	source, names := pragmaOverAlignedBitfields()
	path, compiler := headerFile(t, source), compilerTool(t)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: names})
			if err != nil {
				t.Fatal(err)
			}
			r, err := h.LookupRecord("Named2")
			if err != nil || len(r.MemberAlignments) != len(r.Description.Fields) {
				t.Fatal("missing measured named storage", r, err)
			}
			if target.os == "windows" {
				if r.MemberAlignments[1] != 16 || r.MemberAlignments[2] != 0 || r.Layout.Offsets[1] != 16 || r.Layout.Offsets[2] != 16 {
					t.Fatal("unit start/shared alignment", r)
				}
				padding, _ := h.LookupRecord("Anonymous2")
				if padding.Padding[0].StorageAlignment != 0 || padding.Padding[1].StorageAlignment != 16 || padding.Padding[2].StorageAlignment != 32 || padding.Padding[4].StorageAlignment != 0 {
					t.Fatal("unnamed starts/active/ignored boundaries", padding)
				}
				padding.Padding[1].StorageAlignment = 1
				fresh, _ := h.LookupRecord("Anonymous2")
				if fresh.Padding[1].StorageAlignment != 16 {
					t.Fatal("padding storage snapshot is shared")
				}
			}
			generated, err := h.CgoSource("bindings", "Bindings")
			if err != nil || !strings.Contains(string(generated), "MemberAlignments:") {
				t.Fatal("measured alignment persistence", err)
			}
			if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
				t.Fatal("packed bits accepted without a C bridge")
			}
		})
	}
}

func TestPragmaBitfieldOverAlignmentBridgeMSVCAndMinGW(t *testing.T) {
	source, names := pragmaOverAlignedBitfields()
	testPackedBitfieldBridgeMSVCAndMinGW(t, source, names...)
}

func TestPragmaBitfieldStorageMetadataValidation(t *testing.T) {
	source, names := pragmaOverAlignedBitfields()
	h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: compilerTool(t), Target: "x86_64-pc-windows-msvc", Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []func(*Record){
		func(r *Record) { r.MemberAlignments = nil },
		func(r *Record) { r.MemberAlignments[0] = 0 },
		func(r *Record) { r.MemberAlignments[1] = 64 },
		func(r *Record) { r.MemberAlignments[2] = 16 },
	} {
		r, _ := h.LookupRecord("Named2")
		mutation(&r)
		direct, err := newDirectTypes([]Record{r}, "Bindings")
		if err != nil {
			continue
		}
		mapper := cgoTypes{directTypes: direct, cPrefix: "test", msLayout: true}
		var output bytes.Buffer
		if err := mapper.writePragmaBitfields(&output, 0); err == nil {
			t.Fatal("invalid named storage metadata accepted", r.MemberAlignments)
		}
	}
	for _, mutation := range []func(*Record){
		func(r *Record) { r.Padding[0].StorageAlignment = 16 },
		func(r *Record) { r.Padding[1].StorageAlignment = 0 },
		func(r *Record) { r.Padding[2].StorageAlignment = 3 },
		func(r *Record) { r.Padding[2].StorageAlignment = 128 },
	} {
		r, _ := h.LookupRecord("Anonymous2")
		mutation(&r)
		if err := validRecordLayout(r); err != nil {
			continue
		}
		direct, err := newDirectTypes([]Record{r}, "Bindings")
		if err != nil {
			t.Fatal(err)
		}
		mapper := cgoTypes{directTypes: direct, cPrefix: "test", msLayout: true}
		var output bytes.Buffer
		if err := mapper.writePragmaBitfields(&output, 0); err == nil {
			t.Fatal("invalid unnamed storage metadata accepted", r.Padding)
		}
	}
}
