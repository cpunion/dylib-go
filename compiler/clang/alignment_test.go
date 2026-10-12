package clang

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestAlignedRecordBridgesAcrossTargets(t *testing.T) {
	path := headerFile(t, `typedef struct __attribute__((aligned(32))) { char tag; double value; } Aligned;
typedef struct { char tag; _Alignas(16) double values[2]; int count; } Fields;
typedef struct __attribute__((packed,aligned(32))) { char tag; double value; short tail; } Packed;
typedef struct { Aligned values[2]; Fields fields[2]; Packed packed; } Outer;
Aligned echo(Aligned); Fields fields(Fields); Packed packed(Packed); Outer nest(Outer);
typedef Aligned (*Entry)(Aligned); Entry factory(void); Aligned apply(Entry,Aligned);`)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			compiler := compilerTool(t)
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: []string{"echo", "fields", "packed", "nest", "factory", "apply"}})
			if err != nil {
				t.Fatal(err)
			}
			a, err := h.LookupRecord("Aligned")
			valueOffset := uint64(8)
			if target.os == "linux" && target.arch == "386" {
				valueOffset = 4
			}
			if err != nil || a.Aligned != 32 || !reflect.DeepEqual(a.Layout, abi.Layout{Size: 32, Alignment: 32, Offsets: []uint64{0, valueOffset}}) {
				t.Fatal("record alignment:", a, err)
			}
			f, err := h.LookupRecord("Fields")
			if err != nil || !reflect.DeepEqual(f.FieldAlignments, []FieldAlignment{{Position: 1, Aligned: 16}}) || !reflect.DeepEqual(f.Layout, abi.Layout{Size: 48, Alignment: 16, Offsets: []uint64{0, 16, 32}}) {
				t.Fatal("field alignment:", f, err)
			}
			f.FieldAlignments[0].Aligned = 64
			fresh, err := h.LookupRecord("Fields")
			if err != nil || fresh.FieldAlignments[0].Aligned != 16 {
				t.Fatal("alignment snapshot aliases header:", fresh, err)
			}
			p, err := h.LookupRecord("Packed")
			if err != nil || !p.Packed || p.Aligned != 32 || !reflect.DeepEqual(p.Layout, abi.Layout{Size: 32, Alignment: 32, Offsets: []uint64{0, 1, 9}}) {
				t.Fatal("packed alignment:", p, err)
			}
			source, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"Aligned: 32", "FieldAlignments:", "__attribute__((aligned(32)))", "__attribute__((packed,aligned(32)))", "f1[2] __attribute__((aligned(16)))", "_record3_transport"} {
				if !strings.Contains(string(source), marker) {
					t.Fatalf("missing alignment bridge %s", marker)
				}
			}
			text := string(source)
			start := strings.Index(text, "/*") + 2
			end := strings.Index(text[start:], "*/") + start
			if _, err := runCompiler(context.Background(), compiler, []string{"--target=" + target.triple, "-ffreestanding", "-x", "c", "-std=c17", "-fsyntax-only", "-"}, text[start:end], maxDiagnostics); err != nil {
				t.Fatal(err)
			}
			if target.arch != "386" {
				if _, err := h.LLGoSource("bindings", "Bindings"); err == nil || !strings.Contains(err.Error(), "requires CgoSource") {
					t.Fatal("aligned direct adapter accepted:", err)
				}
			}
			if err := h.Target.CheckHost(); err == nil {
				if _, err := h.ForHost("echo"); err == nil || !strings.Contains(err.Error(), "compiled C adapter") {
					t.Fatal("aligned dynamic declaration accepted:", err)
				}
			}
		})
	}
}

func TestAlignmentAttributeSpellings(t *testing.T) {
	for _, declaration := range []string{
		`struct __attribute__((aligned(8),aligned(16))) S { int value; };`,
		`struct __declspec(align(16)) S { int value; };`,
		`struct S { int value __attribute__((aligned(8),aligned(16))); };`,
		`struct S { _Alignas(16) int value; };`,
		`struct S { __declspec(align(16)) int value; };`,
	} {
		h, err := Parse(context.Background(), headerFile(t, declaration+` struct S echo(struct S);`), Options{Compiler: compilerTool(t), Flags: []string{"-fms-extensions"}, Functions: []string{"echo"}})
		if err != nil {
			t.Fatal(declaration, err)
		}
		r := h.Records[0]
		if r.Layout.Alignment != 16 || r.Aligned != 16 && r.fieldAlignment(0) != 16 {
			t.Fatal("alignment spelling lost:", declaration, r)
		}
	}
}

func TestInvalidAlignmentMetadata(t *testing.T) {
	base := Record{Name: "S", Description: abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "value", Type: abi.TypeDesc{Type: abi.I32}}}}, Layout: abi.Layout{Size: 16, Alignment: 16, Offsets: []uint64{0}}}
	for _, fields := range [][]FieldAlignment{{{Position: -1, Aligned: 16}}, {{Position: 1, Aligned: 16}}, {{Aligned: 0}}, {{Aligned: 3}}, {{Aligned: 131072}}, {{Aligned: 16}, {Aligned: 16}}} {
		r := base
		r.FieldAlignments = fields
		if err := (&Header{Records: []Record{r}}).validateDescriptions(); err == nil {
			t.Fatal("invalid field alignments accepted:", fields)
		}
	}
	for _, value := range []uint64{3, 131072} {
		r := base
		r.Aligned = value
		if err := (&Header{Records: []Record{r}}).validateDescriptions(); err == nil {
			t.Fatal("invalid record alignment accepted:", value)
		}
	}
	for _, offsets := range [][]uint64{nil, {8}} {
		r := base
		r.FieldAlignments = []FieldAlignment{{Aligned: 16}}
		r.Layout.Offsets = offsets
		if err := validRecordLayout(r); err == nil {
			t.Fatal("invalid aligned layout accepted:", offsets)
		}
	}
	r := base
	r.Aligned = 32
	if err := validRecordLayout(r); err == nil {
		t.Fatal("record alignment larger than native layout accepted")
	}
}

func TestZeroAlignmentRequest(t *testing.T) {
	h, err := Parse(context.Background(), headerFile(t, `typedef struct { _Alignas(0) int value; } S; S echo(S);`), Options{Compiler: compilerTool(t), Functions: []string{"echo"}})
	if err != nil || len(h.Records) != 1 || h.Records[0].requiresCAdapter() {
		t.Fatal("zero alignment request should have no effect:", h, err)
	}
}

func TestAlignedRecordAmbiguousLayouts(t *testing.T) {
	h, err := Parse(context.Background(), headerFile(t, `typedef struct { double first; double second; } Plain;
typedef struct __attribute__((aligned(16))) { double first; double second; } Aligned;
Plain plain(Plain); Aligned aligned(Aligned);`), Options{Compiler: compilerTool(t), Functions: []string{"plain", "aligned"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.CgoSource("bindings", "Bindings"); err == nil || !strings.Contains(err.Error(), "ambiguous native layouts") {
		t.Fatal("indistinguishable logical records with different alignments accepted:", err)
	}
}

func TestAlignedArgumentLowering(t *testing.T) {
	path := headerFile(t, `typedef struct { double value; } Natural;
typedef struct __attribute__((aligned(8))) { int value; } Required;
typedef struct { Required values[2]; int count; } Nested;
Natural natural(Natural); Required required(Required); Nested nested(Nested);
typedef Required (*Entry)(Required); Entry factory(void); Required apply(Entry,Required);
Required variable(Required,...);`)
	for _, triple := range []string{"i686-pc-windows-msvc", "i686-w64-windows-gnu", "i686-unknown-linux-gnu"} {
		h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: triple, Functions: []string{"natural", "required", "nested", "factory", "apply", "variable"}})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range h.Records {
			want := strings.Contains(triple, "msvc") && r.Name != "Natural"
			if r.IndirectArgument != want {
				t.Fatal("required alignment lowering:", triple, r)
			}
			if r.IndirectResult != (want && r.Name == "Nested") {
				t.Fatal("aligned register/hidden result lowering:", triple, r)
			}
		}
		h.Functions[5].Signature.Args = append(h.Functions[5].Signature.Args, abi.Struct)
		h.Functions[5].Signature.ArgTypes = append(h.Functions[5].Signature.ArgTypes, h.Records[1].Description)
		source, err := h.CgoSource("bindings", "Bindings")
		if err != nil {
			t.Fatal(err)
		}
		text := string(source)
		if strings.Contains(triple, "msvc") {
			for _, marker := range []string{"IndirectArgument: true", "_record1_argument", "_arg(native0)", "_arg(native1)", "_record1_from_transport(p1)"} {
				if !strings.Contains(text, marker) {
					t.Fatal("missing native argument conversion:", marker)
				}
			}
		} else if strings.Contains(text, "_arg(native") {
			t.Fatal("GNU/Linux prototype rewritten for MSVC")
		}
	}
}

func TestOrdinaryFastcallResultLowering(t *testing.T) {
	path := headerFile(t, `typedef struct { float left; float right; int marker; } Plain;
typedef Plain (__attribute__((fastcall)) *Entry)(double,Plain,int);
Entry factory(void); Plain apply(Entry,Plain);`)
	for _, triple := range []string{"i686-pc-windows-msvc", "i686-w64-windows-gnu"} {
		h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: triple, Functions: []string{"factory", "apply"}})
		if err != nil {
			t.Fatal(err)
		}
		want := strings.Contains(triple, "msvc")
		if len(h.Records) != 1 || h.Records[0].IndirectArgument || h.Records[0].IndirectResult != want || h.Records[0].requiresCAdapter() != want {
			t.Fatal("ordinary fastcall result lowering:", triple, h.Records)
		}
		source, err := h.CgoSource("bindings", "Bindings")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(source), "_record0_result_pointer") != want {
			t.Fatal("ordinary fastcall native prototype:", triple)
		}
	}
}

func TestIndirectRecordIRProbe(t *testing.T) {
	for _, test := range []struct {
		parameters string
		want       bool
	}{
		{"ptr noundef %0", true},
		{"%struct.Record* noundef %0", true},
		{"ptr noundef byval(%struct.Record) align 4 %0", false},
		{"double %0", false},
		{"double %0, ptr %1", false},
	} {
		got, err := indirectRecordArgument("define void @probe("+test.parameters+") #0 {\n", "probe")
		if err != nil || got != test.want {
			t.Fatal(test, got, err)
		}
	}
	for _, ir := range []string{"", "declare void @probe(ptr)", "define void @probe(ptr noundef %0"} {
		if _, err := indirectRecordArgument(ir, "probe"); err == nil {
			t.Fatal("missing/malformed argument probe accepted:", ir)
		}
	}
}

func TestCompilerEvaluatedAlignmentRequests(t *testing.T) {
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			compiler := compilerTool(t)
			doubleAlignment := 8
			if target.os == "linux" && target.arch == "386" {
				doubleAlignment = 4
			}
			pairs := [][2]string{
				{`typedef struct __attribute__((aligned)) { char tag; double value; } S;`, `typedef struct __attribute__((aligned(16))) { char tag; double value; } S;`},
				{`typedef struct __attribute__((packed,aligned)) { char tag; double value; } S;`, `typedef struct __attribute__((packed,aligned(16))) { char tag; double value; } S;`},
				{`typedef struct { char tag; double values[2] __attribute__((aligned)); } S;`, `typedef struct { char tag; double values[2] __attribute__((aligned(16))); } S;`},
				{`typedef struct { unsigned low:3; int barrier; signed high:7 __attribute__((aligned)); char tail; } S;`, `typedef struct { unsigned low:3; int barrier; signed high:7 __attribute__((aligned(16))); char tail; } S;`},
				{`typedef struct { char tag; _Alignas(double) int value; } S;`, fmt.Sprintf(`typedef struct { char tag; _Alignas(%d) int value; } S;`, doubleAlignment)},
				{`typedef struct __attribute__((aligned(32))) { char token; } Alignment; typedef struct { char tag; _Alignas(Alignment) double values[2]; } S;`, `typedef struct { char tag; _Alignas(32) double values[2]; } S;`},
				{`typedef struct { char tag; _Alignas(double) int value; _Alignas(double[2]) int count; } S;`, fmt.Sprintf(`typedef struct { char tag; _Alignas(%d) int value; _Alignas(%d) int count; } S;`, doubleAlignment, doubleAlignment)},
			}
			// Share compiler invocations across spellings on each target. Generating
			// isolated adapters still checks each native layout independently: some
			// cases deliberately share logical fields with different C attributes.
			var original, explicit strings.Builder
			var names []string
			for i, pair := range pairs {
				record, name := fmt.Sprintf("S%d", i), fmt.Sprintf("echo%d", i)
				names = append(names, name)
				fmt.Fprintf(&original, "%s %s %s(%s);\n", strings.ReplaceAll(pair[0], " S;", " "+record+";"), record, name, record)
				fmt.Fprintf(&explicit, "%s %s %s(%s);\n", strings.ReplaceAll(pair[1], " S;", " "+record+";"), record, name, record)
			}
			parse := func(source string) *Header {
				t.Helper()
				h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: compiler, Target: target.triple, Functions: names})
				if err != nil {
					t.Fatal(err)
				}
				return h
			}
			actual, numeric := parse(original.String()), parse(explicit.String())
			if !reflect.DeepEqual(actual.Records, numeric.Records) {
				t.Fatalf("compiler-evaluated requests differ from numeric requests:\n%+v\n%+v", actual.Records, numeric.Records)
			}
			var cSource strings.Builder
			for i, record := range actual.Records {
				isolated := &Header{Target: actual.Target, Records: []Record{record}, Functions: []Declaration{actual.Functions[i]}}
				source, err := isolated.CgoSource("bindings", fmt.Sprintf("Bindings%d", i))
				if err != nil {
					t.Fatal(err)
				}
				text := string(source)
				start := strings.Index(text, "/*") + 2
				end := strings.Index(text[start:], "*/") + start
				cSource.WriteString(text[start:end])
				if _, err := isolated.LLGoSource("bindings", "Bindings"); err == nil {
					t.Fatal("attributed direct adapter accepted")
				}
			}
			if _, err := runCompiler(context.Background(), compiler, []string{"--target=" + target.triple, "-ffreestanding", "-x", "c", "-std=c17", "-fsyntax-only", "-"}, cSource.String(), maxDiagnostics); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMissingCompilerAlignmentProbes(t *testing.T) {
	r := &typeResolver{}
	if _, err := r.recordAlignment(astNode{Inner: []astNode{{}}}); err == nil {
		t.Fatal("missing maximum alignment accepted")
	}
	field, attr := astNode{Name: "value"}, astNode{Kind: "AlignedAttr"}
	if _, err := r.memberAlignment("S", field, attr); !errors.Is(err, errRecordFieldTypes) {
		t.Fatal("missing member probe did not request compiler types:", err)
	}
	r.fieldProbes = map[recordField]recordFieldProbe{}
	if _, err := r.memberAlignment("S", field, attr); err == nil {
		t.Fatal("absent member alignment accepted")
	}
	keys := map[string]recordField{"dylib_field_probe_0": {"S", "value"}}
	for _, input := range []string{
		`{"inner":[{"kind":"TypedefDecl","name":"dylib_field_probe_0"}]}`,
		`{"inner":[{"kind":"EnumDecl","inner":[{"kind":"EnumConstantDecl","name":"dylib_field_probe_0_alignment","inner":[{"kind":"ConstantExpr","value":"8"}]}]}]}`,
		`{"inner":[{"kind":"TypedefDecl","name":"dylib_field_probe_0"},{"kind":"EnumDecl","inner":[{"kind":"EnumConstantDecl","name":"dylib_field_probe_0_alignment","inner":[{"kind":"ConstantExpr","value":"3"}]}]}]}`,
	} {
		if _, err := decodeFieldProbes([]byte(input), keys); err == nil {
			t.Fatal("incomplete/invalid compiler field probe accepted")
		}
	}
}
