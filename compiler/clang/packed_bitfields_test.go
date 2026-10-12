package clang

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCompilerPackedBitfieldMembers(t *testing.T) {
	path := headerFile(t, `
typedef struct { char tag; unsigned a:3 __attribute__((packed)); signed b:5; _Bool on:1 __attribute__((packed)); unsigned long long wide:40 __attribute__((packed)); double value; } Mixed;
typedef struct { unsigned a:3 __attribute__((packed)); signed b:5 __attribute__((packed)); } Tiny;
typedef struct { char tag; unsigned a:3 __attribute__((packed,aligned(2))); signed b:5; double value; } Aligned;
typedef struct { char tag; unsigned a:31 __attribute__((packed)); unsigned b:31; unsigned c:2 __attribute__((packed)); short tail; } Units;
typedef struct { char tag; unsigned a:3; signed b:5 __attribute__((packed)); short tail; } Later;
typedef struct { char marker; unsigned low:3 __attribute__((packed,aligned(16))); signed high:5; double real; } Overaligned;
typedef struct { unsigned a:3 __attribute__((packed)); unsigned :0; signed b:7 __attribute__((packed)); double value; } Gaps;
typedef struct { char marker; unsigned lo:3 __attribute__((packed)); unsigned :4; unsigned long long :0; unsigned :0; signed hi:5 __attribute__((packed)); unsigned :0; char tail; } Boundaries;
typedef struct { unsigned :3; unsigned lo:5 __attribute__((packed)); unsigned :31; unsigned hi:1 __attribute__((packed)); unsigned :0; char last; } Leading;
typedef struct { unsigned lo:3 __attribute__((packed)); unsigned :4; unsigned long long :0; unsigned :0; } Trailing;
typedef struct { char prefix; unsigned lo:3 __attribute__((packed,aligned(16))); unsigned :4; unsigned long long :0; signed hi:5 __attribute__((packed)); unsigned :0; char suffix; } AlignedGaps;
typedef struct { char prefix; unsigned long long all:64 __attribute__((packed)); unsigned long long :0; _Bool flag:1 __attribute__((packed)); unsigned :2; unsigned char :0; signed char byte:7 __attribute__((packed)); unsigned :0; double real; } TypeChanges;
typedef struct { char padding_marker; unsigned :3 __attribute__((packed)); char padding_tail; } PaddingOnly;
typedef struct { char shared_marker; unsigned :3 __attribute__((packed)); unsigned low:5; unsigned :0 __attribute__((packed)); int shared_count; } SharedPadding;
typedef struct { char zero_marker; unsigned low:3 __attribute__((packed)); unsigned :0 __attribute__((packed)); char zero_tail; } PackedZero;
typedef struct { char ignored_marker; unsigned :0 __attribute__((packed)); unsigned :0 __attribute__((packed)); double ignored_real; } IgnoredZero;
typedef struct __attribute__((packed)) { char global_marker; unsigned :3 __attribute__((packed)); signed global_value:5; unsigned long long :0 __attribute__((packed)); double global_real; } GlobalPadding;
PaddingOnly padding_only(PaddingOnly); SharedPadding shared_padding(SharedPadding); PackedZero packed_zero(PackedZero); IgnoredZero ignored_zero(IgnoredZero); GlobalPadding global_padding(GlobalPadding);
Boundaries boundaries(Boundaries); Leading leading(Leading); Trailing trailing(Trailing); AlignedGaps aligned_gaps(AlignedGaps); TypeChanges type_changes(TypeChanges);
Mixed mixed(Mixed); Tiny tiny(Tiny); Aligned aligned(Aligned); Units units(Units); Later later(Later); Overaligned overaligned(Overaligned); Gaps gaps(Gaps);
`)
	compiler := compilerTool(t)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			names := []string{"mixed", "tiny", "aligned", "units", "later", "overaligned", "gaps", "boundaries", "leading", "trailing", "aligned_gaps", "type_changes", "padding_only", "shared_padding", "packed_zero", "ignored_zero", "global_padding"}
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: names})
			if err != nil {
				t.Fatal(err)
			}
			mixed, err := h.LookupRecord("Mixed")
			if err != nil || !reflect.DeepEqual(mixed.PackedFields, []int{1, 3, 4}) {
				t.Fatal("packed member metadata:", mixed, err)
			}
			mixed.PackedFields[0] = 0
			freshMixed, _ := h.LookupRecord("Mixed")
			if freshMixed.PackedFields[0] != 1 {
				t.Fatal("packed member snapshot aliases the header")
			}
			paddingRecord, err := h.LookupRecord("PaddingOnly")
			if err != nil || len(paddingRecord.Padding) != 1 || !paddingRecord.Padding[0].Packed || len(paddingRecord.PackedFields) != 0 {
				t.Fatal("unnamed packing metadata:", paddingRecord, err)
			}
			paddingRecord.Padding[0].Packed = false
			freshPadding, _ := h.LookupRecord("PaddingOnly")
			if !freshPadding.Padding[0].Packed {
				t.Fatal("unnamed packing snapshot aliases the header")
			}
			if target.os == "windows" {
				zero, err := h.LookupRecord("PackedZero")
				if err != nil || len(zero.Padding) != 1 || !zero.Padding[0].Packed || zero.Padding[0].Offset != 5 || zero.Layout.Alignment != 1 || zero.Layout.Size != 6 {
					t.Fatal("packed zero-width boundary:", zero, err)
				}
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
			for i, record := range h.Records {
				name := fmt.Sprintf("dylib_go_%x_record%d", "Bindings", i)
				fmt.Fprintf(&probe, "\n_Static_assert(sizeof(%s)==%d,\"size\");\n_Static_assert(_Alignof(%s)==%d,\"alignment\");\n", name, record.Layout.Size, name, record.Layout.Alignment)
				for j, offset := range record.Layout.Offsets {
					if record.bitfieldAt(j) == nil {
						fmt.Fprintf(&probe, "_Static_assert(offsetof(%s,f%d)==%d,\"offset\");\n", name, j, offset)
					}
				}
				for _, field := range record.Bitfields {
					fmt.Fprintf(&probe, "const %s dylib_bitfield_%d_%d = {.f%d=%s};\n", name, i, field.Position, field.Position, bitfieldValue(record.Description.Fields[field.Position].Type.Type, field.Width))
				}
			}
			triples := []string{target.triple}
			if target.os == "windows" {
				triples = append(triples, strings.Replace(target.triple, "pc-windows-msvc", "w64-windows-gnu", 1))
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
				symbols, err := readBitfieldSymbols(data, target.os)
				if err != nil {
					t.Fatal(err)
				}
				for i, record := range h.Records {
					for _, field := range record.Bitfields {
						value, err := symbols.value(fmt.Sprintf("dylib_bitfield_%d_%d", i, field.Position), record.Layout.Size)
						want := make([]byte, record.Layout.Size)
						copy(want[record.Layout.Offsets[field.Position]:], field.Mask)
						if err != nil || !bytes.Equal(value, want) {
							t.Fatal(triple, record.Name, field.Position, value, want, err)
						}
					}
				}
			}
			if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
				t.Fatal("direct llgo accepted member-packed bitfields")
			}
		})
	}
}
