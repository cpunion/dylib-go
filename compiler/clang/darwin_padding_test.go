package clang

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/ramblelabs/dylib-go/abi"
)

var darwinPlainGroups = []struct {
	name  string
	count int
}{{"mixed", 147}, {"bases", 246}, {"arrays", 96}, {"multiple", 96}}

func darwinPlainHeader(group string) (string, []string) {
	var source strings.Builder
	var names []string
	add := func(body func(string, string) string) {
		name := fmt.Sprintf("Plain%d", len(names))
		fmt.Fprintf(&source, "typedef struct {%s} %s; %s echo_%s(%s);\n", body("first_"+name, "last_"+name), name, name, name, name)
		names = append(names, "echo_"+name)
	}
	ordinary := func(base string, widths []int, pairs [][2]string) {
		for _, pair := range pairs {
			for _, width := range widths {
				for _, mode := range []string{"leading", "interior", "trailing"} {
					add(func(first, last string) string {
						padding := fmt.Sprintf("%s :%d;", base, width)
						switch mode {
						case "leading":
							return fmt.Sprintf("%s %s %s;", padding, pair[1], last)
						case "trailing":
							return fmt.Sprintf("%s %s; %s", pair[0], first, padding)
						default:
							return fmt.Sprintf("%s %s; %s %s %s;", pair[0], first, padding, pair[1], last)
						}
					})
				}
			}
		}
	}
	switch group {
	case "mixed":
		ordinary("unsigned long long", []int{1, 3, 31, 32, 33, 40, 64}, [][2]string{{"float", "signed char"}, {"signed char", "float"}, {"float", "int"}, {"float", "double"}, {"int", "float"}, {"double", "signed char"}, {"signed char", "int"}})
	case "bases":
		for _, base := range []struct {
			kind   string
			widths []int
		}{{"unsigned char", []int{1, 3, 7, 8}}, {"signed char", []int{1, 3, 7, 8}}, {"unsigned short", []int{1, 7, 15, 16}}, {"short", []int{1, 7, 15, 16}}, {"unsigned", []int{1, 7, 31, 32}}, {"int", []int{1, 7, 31, 32}}, {"unsigned long long", []int{1, 3, 31, 32, 33, 40, 63, 64}}, {"long long", []int{1, 3, 31, 32, 33, 40, 63, 64}}, {"_Bool", []int{1}}} {
			ordinary(base.kind, base.widths, [][2]string{{"float", "signed char"}, {"float", "float"}})
		}
	case "arrays":
		for _, kind := range []string{"float", "double"} {
			for _, length := range []int{1, 2, 3, 4} {
				for _, width := range []int{1, 32, 33, 64} {
					for _, mode := range []string{"leading", "interior", "trailing"} {
						add(func(first, last string) string {
							padding := fmt.Sprintf("unsigned long long :%d;", width)
							array := fmt.Sprintf("%s %s[%d];", kind, last, length)
							switch mode {
							case "leading":
								return padding + array
							case "trailing":
								return array + padding
							default:
								return "float " + first + ";" + padding + array
							}
						})
					}
				}
			}
		}
	case "multiple":
		for _, firstType := range []string{"float", "signed char", "int", "double"} {
			for _, lastType := range []string{"float", "signed char", "int", "double"} {
				for _, widths := range [][2]int{{1, 3}, {3, 33}, {31, 40}, {33, 1}, {40, 31}, {64, 64}} {
					add(func(first, last string) string {
						return fmt.Sprintf("unsigned long long :%d; %s %s; unsigned long long :%d; unsigned long long :%d; %s %s; unsigned long long :%d;", widths[0], firstType, first, widths[1], widths[0], lastType, last, widths[1])
					})
				}
			}
		}
	}
	return source.String(), names
}

func TestDarwinPlainPaddingBridge(t *testing.T) {
	compiler := compilerTool(t)
	for _, group := range darwinPlainGroups {
		t.Run(group.name, func(t *testing.T) {
			source, names := darwinPlainHeader(group.name)
			testCompilerBridgeTargetsWithInspection(t, compiler, source, nil, []string{"arm64-apple-macosx11"}, func(t *testing.T, path string, h *Header) {
				if len(h.Records) != group.count {
					t.Fatal("missing plain unnamed cases", len(h.Records), group.count)
				}
				testCompilerRecordCallingABI(t, compiler, path, h)
			}, names...)
		})
	}
}

// Compile the real generated bridges with GCC and call independent Clang
// definitions. A layout assertion alone cannot qualify an aggregate call ABI.
func TestDarwinPlainPaddingGCCCalls(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("native Darwin ARM64 GCC qualification")
	}
	gcc := gccCompiler()
	if gcc == "" {
		if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
			t.Fatal("GNU GCC is required for Darwin unnamed-field qualification")
		}
		t.Skip("GNU GCC is unavailable")
	}
	compiler := compilerTool(t)
	for _, group := range darwinPlainGroups {
		t.Run(group.name, func(t *testing.T) {
			source, names := darwinPlainHeader(group.name)
			path := headerFile(t, source)
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Functions: names})
			if err != nil {
				t.Fatal(err)
			}
			if len(h.Records) != group.count {
				t.Fatal("missing native GCC cases", len(h.Records), group.count)
			}
			generated, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			var producer, caller, checks strings.Builder
			fmt.Fprintf(&producer, "#include <stdint.h>\n#include %q\n", path)
			text := strings.Split(strings.Split(string(generated), "/*")[1], "*/")[0]
			caller.WriteString(text)
			for i, record := range h.Records {
				prefix := fmt.Sprintf("dylib_go_%x", "Bindings")
				native := fmt.Sprintf("%s_record%d", prefix, i)
				fmt.Fprintf(&caller, "_Static_assert(sizeof(%s)==%d && _Alignof(%s)==%d,\"native layout\");\n", native, record.Layout.Size, native, record.Layout.Alignment)
				fmt.Fprintf(&producer, "%s echo_%s(%s v){\n", record.Name, record.Name, record.Name)
				fmt.Fprintf(&checks, "{%s_transport v={0};\n", native)
				var conditions []string
				for j, field := range record.Description.Fields {
					fmt.Fprintf(&caller, "_Static_assert(offsetof(%s,f%d)==%d,\"native offset\");\n", native, j, record.Layout.Offsets[j])
					length := 1
					if field.Type.Type == abi.Array {
						length = field.Type.Len
					}
					for k := 0; k < length; k++ {
						suffix := ""
						if field.Type.Type == abi.Array {
							suffix = fmt.Sprintf("[%d]", k)
						}
						fmt.Fprintf(&producer, "v.%s%s+=2;\n", field.Name, suffix)
						fmt.Fprintf(&checks, "v.f%d%s=20;\n", j, suffix)
						conditions = append(conditions, fmt.Sprintf("v.f%d%s!=22", j, suffix))
					}
				}
				producer.WriteString("return v;}\n")
				// Keep the callee opaque to GCC, as a dynamically resolved
				// address is in production. No cross-unit LTO is involved.
				fmt.Fprintf(&producer, "uintptr_t entry_%d(void){return (uintptr_t)&echo_%s;}\n", i, record.Name)
				fmt.Fprintf(&caller, "extern uintptr_t entry_%d(void);\n", i)
				fmt.Fprintf(&checks, "v=%s_call%d(entry_%d(),&v); if(%s)return %d;}\n", prefix, i, i, strings.Join(conditions, "||"), i+1)
			}
			fmt.Fprintf(&caller, "int main(void){%s return 0;}\n", checks.String())
			testDarwinGCCCaller(t, compiler, gcc, producer.String(), caller.String())
		})
	}
	t.Run("fixture", func(t *testing.T) { testDarwinPlainFixtureGCC(t, compiler, gcc) })
}

func testDarwinPlainFixtureGCC(t *testing.T, compiler, gcc string) {
	dir, err := filepath.Abs("../../examples/plainpadding")
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	names := strings.Fields(string(read("testdata/functions.txt")))
	h, err := Parse(context.Background(), filepath.Join(dir, "testdata/exports.h"), Options{Compiler: compiler, Functions: names})
	if err != nil || len(h.Records) != 9 {
		t.Fatal("native GCC fixture metadata", err)
	}
	generated := string(read("generated_darwin_arm64.go"))
	var caller, producer strings.Builder
	caller.WriteString(strings.Split(strings.Split(generated, "/*")[1], "*/")[0])
	fmt.Fprintf(&producer, "#include <stdint.h>\n#include %q\n", filepath.Join(dir, "testdata/exports.c"))
	for _, record := range names {
		fmt.Fprintf(&producer, "uintptr_t entry_%s(void){return (uintptr_t)&%s;}\n", record, record)
		fmt.Fprintf(&caller, "extern uintptr_t entry_%s(void);\n", record)
	}
	for i, r := range h.Records {
		name := fmt.Sprintf("dylib_go_%x_record%d", "Bindings", i)
		fmt.Fprintf(&caller, "_Static_assert(sizeof(%s)==%d && _Alignof(%s)==%d,\"fixture layout\");\n", name, r.Layout.Size, name, r.Layout.Alignment)
		for j, offset := range r.Layout.Offsets {
			fmt.Fprintf(&caller, "_Static_assert(offsetof(%s,f%d)==%d,\"fixture offset\");\n", name, j, offset)
		}
	}
	caller.WriteString("#line 1 \"gcc_calls.c\"\n")
	caller.Write(read("testdata/gcc_calls.c"))
	testDarwinGCCCaller(t, compiler, gcc, producer.String(), caller.String())
}

func testDarwinGCCCaller(t *testing.T, compiler, gcc, producer, caller string) {
	t.Helper()
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			object, binary := filepath.Join(t.TempDir(), "producer.o"), filepath.Join(t.TempDir(), "caller")
			if _, err := runCompiler(context.Background(), compiler, []string{"-x", "c", "-std=c17", opt, "-c", "-o", object, "-"}, producer, maxDiagnostics); err != nil {
				t.Fatal(err)
			}
			if _, err := runCompiler(context.Background(), gcc, []string{"-x", "c", "-std=c17", opt, "-o", binary, "-", "-x", "none", object}, caller, maxDiagnostics); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(binary).CombinedOutput(); err != nil {
				t.Fatalf("Clang producer/GCC caller: %v\n%s", err, output)
			}
		})
	}
}

func TestDarwinPlainPaddingMetadata(t *testing.T) {
	h, err := Parse(context.Background(), headerFile(t, "typedef struct {float first; unsigned long long :33; signed char last;} V; V echo(V);"), Options{Compiler: compilerTool(t), Target: "arm64-apple-macosx11", Functions: []string{"echo"}})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := h.LookupRecord("V")
	if _, err := h.CgoSource("bindings", "Bindings"); err != nil {
		t.Fatal(err)
	}
	after, _ := h.LookupRecord("V")
	if !reflect.DeepEqual(before, after) || after.Aligned != 0 || after.Padding[0].Width != 33 {
		t.Fatal("private byte facade changed original metadata")
	}
	for _, offset := range []uint64{1, before.Layout.Size - 1} {
		bad := *h
		r, _ := h.LookupRecord("V")
		r.Layout.Offsets[1] = offset
		if offset == before.Layout.Size-1 {
			// This ordinary field no longer fits inside the measured record.
			r.Description.Fields[1].Type = abi.TypeDesc{Type: abi.F64}
			bad.Functions = append([]Declaration(nil), h.Functions...)
			bad.Functions[0].Signature.ArgTypes = []abi.TypeDesc{r.Description}
			result := r.Description
			bad.Functions[0].Signature.ResultType = &result
		}
		bad.Records = []Record{r}
		if _, err := bad.CgoSource("bindings", "Bindings"); err == nil {
			t.Fatal("overlapping/out-of-range native fields accepted")
		}
	}
}
