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
Mixed mixed(Mixed); Tiny tiny(Tiny); Aligned aligned(Aligned); Units units(Units); Later later(Later); Overaligned overaligned(Overaligned); Gaps gaps(Gaps);
`)
	compiler := compilerTool(t)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			names := []string{"mixed", "tiny", "aligned", "units", "later", "overaligned"}
			if target.os != "windows" {
				names = append(names, "gaps")
			}
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: names})
			if err != nil {
				t.Fatal(err)
			}
			mixed, err := h.LookupRecord("Mixed")
			if err != nil || !reflect.DeepEqual(mixed.PackedFields, []int{1, 3, 4}) {
				t.Fatal("packed member metadata:", mixed, err)
			}
			mixed.PackedFields[0] = 0
			if h.Records[0].PackedFields[0] != 1 {
				t.Fatal("packed member snapshot aliases the header")
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

func TestMemberPackedBitfieldPaddingBoundary(t *testing.T) {
	path := headerFile(t, `typedef struct { unsigned a:3 __attribute__((packed)); unsigned :0; int value; } Bits; Bits echo(Bits);`)
	for _, target := range targets {
		if target.os != "windows" {
			continue
		}
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"echo"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.CgoSource("bindings", "Bindings"); err == nil || !strings.Contains(err.Error(), "unnamed padding") {
				t.Fatal("unqualified mixed padding accepted:", err)
			}
		})
	}
	testPackedBitfieldBridgeMSVCAndMinGW(t, `typedef struct __attribute__((packed)) { unsigned a:3 __attribute__((packed)); unsigned :0; int value; } Bits; Bits echo(Bits);`)
}
