package clang

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestCgoMSVCFloatReturnABI(t *testing.T) {
	path := headerFile(t, `
typedef struct { float plain; } Plain;
#pragma pack(push,2)
typedef struct { float single; unsigned :0 __attribute__((aligned(8))); } Single;
typedef struct { double wide; unsigned :0 __attribute__((aligned(16))); } Wide;
typedef struct { float items[1]; } Array;
typedef struct { Plain nested; } Nested;
#pragma pack(pop)
typedef struct __attribute__((packed)) { float packed; } Packed;
typedef struct { float member __attribute__((packed)); } Member;
typedef struct __attribute__((packed)) { float zero; unsigned :0 __attribute__((aligned(8))); } PackedZero;
Plain p(Plain); Single s(Single); Wide w(Wide); Array a(Array); Nested n(Nested); Packed k(Packed); Member m(Member); PackedZero z(PackedZero);
`)
	compiler := compilerTool(t)
	for _, producer := range []string{"i686-pc-windows-msvc", "i686-w64-windows-gnu"} {
		t.Run(producer, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: producer, Functions: []string{"p", "s", "w", "a", "n", "k", "m", "z"}})
			if err != nil {
				t.Fatal(err)
			}
			generated, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			var native, bridge strings.Builder
			fmt.Fprintf(&native, "#include %q\n", filepath.ToSlash(path))
			text := string(generated)
			bridge.WriteString(strings.Split(strings.Split(text, "/*")[1], "*/")[0])
			for i, record := range h.Records {
				fmt.Fprintf(&native, "%s call_%d(%s v){return v;}\n", record.Name, i, record.Name)
				name := fmt.Sprintf("dylib_go_%x_record%d", "Bindings", i)
				fmt.Fprintf(&bridge, "_Static_assert(sizeof(%s)==%d,\"size\");\n_Static_assert(_Alignof(%s)==%d,\"alignment\");\n", name, record.Layout.Size, name, record.Layout.Alignment)
				fmt.Fprintf(&bridge, "%s call_%d(%s v){return v;}\n", name, i, name)
			}
			compile := func(triple, source string) string {
				t.Helper()
				args := []string{"--target=" + triple, "-ffreestanding", "-x", "c", "-std=c17", "-O0", "-S", "-emit-llvm", "-o", "-", "-"}
				ir, err := runCompiler(context.Background(), compiler, args, source, maxASTSize)
				if err != nil {
					t.Fatal(err)
				}
				return string(ir)
			}
			nativeIR, bridgeIR := compile(producer, native.String()), compile("i686-w64-windows-gnu", bridge.String())
			entries := func(ir string) []string {
				var result []string
				for _, line := range strings.Split(ir, "\n") {
					if strings.HasPrefix(line, "define ") && strings.Contains(line, "@call_") {
						// Plain scalar fields can expand into a scalar stack
						// argument in MSVC IR and into byval bytes in MinGW IR.
						// Native CI calls qualify those stack representations;
						// compare the return registers independently here.
						line, _, _ = strings.Cut(line, " @call_")
						result = append(result, line)
					}
				}
				return result
			}
			want, got := entries(nativeIR), entries(bridgeIR)
			if len(want) != len(h.Records) || strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("native ABI:\n%s\nbridge ABI:\n%s", strings.Join(want, "\n"), strings.Join(got, "\n"))
			}
		})
	}
}
