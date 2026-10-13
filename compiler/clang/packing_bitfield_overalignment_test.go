package clang

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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
		{"TrailingTiny", "", "unsigned low:3 __attribute__((aligned(8))); signed high:5 __attribute__((aligned(32))); unsigned long long :0 __attribute__((aligned(16)));"},
		{"TrailingFloat", "", "float left; float right; float middle; unsigned long long :0 __attribute__((aligned(16)));"},
		{"TrailingUnaligned", "", "char tag; double real; unsigned long long :0 __attribute__((aligned(16)));"},
		{"TrailingSingleFloat", "", "float left; unsigned :0 __attribute__((aligned(8)));"},
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

func TestPragmaBitfieldOverAlignmentGNUBridge(t *testing.T) {
	source, names := pragmaOverAlignedBitfields()
	var producers []string
	for _, target := range targets {
		if target.os != "windows" {
			producers = append(producers, target.triple)
		}
	}
	testBitfieldBridgeTargets(t, source, nil, producers, names...)
}

func TestPragmaBitfieldOverAlignmentGNUCallABI(t *testing.T) {
	source, names := pragmaOverAlignedBitfields()
	path, compiler := headerFile(t, source), compilerTool(t)
	for _, target := range targets {
		if target.os == "windows" {
			continue
		}
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: names})
			if err != nil {
				t.Fatal(err)
			}
			generated, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			text := string(generated)
			start := strings.Index(text, "/*") + 2
			end := strings.Index(text[start:], "*/") + start
			var original, bridge strings.Builder
			fmt.Fprintf(&original, "#include %q\n", filepath.ToSlash(path))
			bridge.WriteString(text[start:end])
			for i, record := range h.Records {
				fmt.Fprintf(&original, "%s call_%d(%s value){return value;}\n", record.Name, i, record.Name)
				name := fmt.Sprintf("dylib_go_%x_record%d", "Bindings", i)
				fmt.Fprintf(&bridge, "%s call_%d(%s value){return value;}\n", name, i, name)
			}
			args := []string{"--target=" + target.triple, "-ffreestanding", "-x", "c", "-std=c17", "-O0", "-S", "-emit-llvm", "-o", "-", "-"}
			nativeIR, err := runCompiler(context.Background(), compiler, args, original.String(), maxASTSize)
			if err != nil {
				t.Fatal(err)
			}
			bridgeIR, err := runCompiler(context.Background(), compiler, args, bridge.String(), maxASTSize)
			if err != nil {
				t.Fatal(err)
			}
			// LLVM record names are local to each source; retain physical
			// result/argument types, attributes and alignments for comparison.
			normalize := regexp.MustCompile(`%struct\.[a-zA-Z0-9_.]+`)
			entry := func(ir, name string) string {
				for _, line := range strings.Split(ir, "\n") {
					if strings.HasPrefix(line, "define ") && strings.Contains(line, "@"+name+"(") {
						line, _, _ = strings.Cut(strings.TrimSuffix(line, " {"), " #")
						return normalize.ReplaceAllString(line, "%record")
					}
				}
				return ""
			}
			for i, record := range h.Records {
				name := fmt.Sprintf("call_%d", i)
				want, got := entry(string(nativeIR), name), entry(string(bridgeIR), name)
				if want == "" || got != want {
					t.Fatalf("%s ABI: got %s; want %s", record.Name, got, want)
				}
			}
		})
	}
}

func TestPragmaBitfieldOverAlignmentGCCBridge(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("MS layout has separate MinGW bridge coverage")
	}
	candidates := []string{"gcc"}
	if fc := os.Getenv("DYLIB_FC"); strings.Contains(filepath.Base(fc), "gfortran") {
		candidates = append(candidates, filepath.Join(filepath.Dir(fc), strings.Replace(filepath.Base(fc), "gfortran", "gcc", 1)))
	}
	candidates = append(candidates, "gcc-16", "gcc-15", "gcc-14")
	var gcc string
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		version, err := exec.Command(path, "--version").CombinedOutput()
		if err == nil && bytes.Contains(version, []byte("Free Software Foundation")) {
			gcc = path
			break
		}
	}
	if gcc == "" {
		t.Skip("GNU GCC is unavailable")
	}
	source, names := pragmaOverAlignedBitfields()
	h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: compilerTool(t), Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := h.CgoSource("bindings", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	start := strings.Index(text, "/*") + 2
	end := strings.Index(text[start:], "*/") + start
	var probe strings.Builder
	probe.WriteString(text[start:end])
	for i, record := range h.Records {
		name := fmt.Sprintf("dylib_go_%x_record%d", "Bindings", i)
		fmt.Fprintf(&probe, "\n_Static_assert(sizeof(%s)==%d,\"size %s\");\n_Static_assert(_Alignof(%s)==%d,\"alignment %s\");\n", name, record.Layout.Size, record.Name, name, record.Layout.Alignment, record.Name)
		for j, offset := range record.Layout.Offsets {
			if record.bitfieldAt(j) == nil {
				fmt.Fprintf(&probe, "_Static_assert(offsetof(%s,f%d)==%d,\"offset %s\");\n", name, j, offset, record.Name)
			}
		}
		for _, field := range record.Bitfields {
			fmt.Fprintf(&probe, "const %s dylib_bitfield_%d_%d = {.f%d=%s};\n", name, i, field.Position, field.Position, bitfieldValue(record.Description.Fields[field.Position].Type.Type, field.Width))
		}
	}
	object := filepath.Join(t.TempDir(), "gcc-bridge.o")
	args := []string{"-ffreestanding", "-x", "c", "-std=c17", "-c", "-o", object, "-"}
	if h.Target.Arch == "386" {
		args = append(args, "-m32")
	}
	if _, err := runCompiler(context.Background(), gcc, args, probe.String(), maxDiagnostics); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(object)
	if err != nil {
		t.Fatal(err)
	}
	symbols, err := readBitfieldSymbols(data, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	for i, record := range h.Records {
		for _, field := range record.Bitfields {
			value, err := symbols.value(fmt.Sprintf("dylib_bitfield_%d_%d", i, field.Position), record.Layout.Size)
			want := make([]byte, record.Layout.Size)
			copy(want[record.Layout.Offsets[field.Position]:], field.Mask)
			if err != nil || !bytes.Equal(value, want) {
				t.Fatal(record.Name, field.Position, value, want, err)
			}
		}
	}
}

func TestPragmaBitfieldSmallGNUInteriorGap(t *testing.T) {
	path := headerFile(t, `
#pragma pack(push,2)
typedef struct { float left; unsigned :0 __attribute__((aligned(8))); float right; } Value;
#pragma pack(pop)
Value echo(Value);
`)
	for _, triple := range []string{"x86_64-linux-gnu", "x86_64-apple-macosx11", "aarch64-linux-gnu", "x86_64-pc-windows-msvc"} {
		t.Run(triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: triple, Functions: []string{"echo"}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = h.CgoSource("bindings", "Bindings")
			if strings.HasPrefix(triple, "x86_64-") && !strings.Contains(triple, "windows") {
				if err == nil || !strings.Contains(err.Error(), "implicit-padding C bridge") {
					t.Fatal("unqualified GCC register classification accepted", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
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
	gnu, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: compilerTool(t), Target: "x86_64-linux-gnu", Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []func(*Record){
		func(r *Record) { r.Padding[2].Gap = r.Padding[2].Offset + 1 },
		func(r *Record) { r.Padding[1].Gap = 1 },
		func(r *Record) { r.Padding[2].Aligned = r.MaxFieldAlignment },
	} {
		r, _ := gnu.LookupRecord("Anonymous2")
		mutation(&r)
		if err := validRecordLayout(r); err == nil {
			t.Fatal("invalid GNU boundary gap accepted", r.Padding)
		}
	}
}
