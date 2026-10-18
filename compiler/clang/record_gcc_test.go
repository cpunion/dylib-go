package clang

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func gccCompiler() string {
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
	return gcc
}

func testCompilerRecordGCCBridge(t *testing.T, compiler, source string, names []string) {
	t.Helper()
	gcc := gccCompiler()
	if gcc == "" {
		t.Skip("GNU GCC is unavailable")
	}
	h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: compiler, Functions: names})
	if errors.Is(err, errTypedefABI) {
		t.Log("compiler rejects legacy SysV typedef ABI:", err)
		return
	}
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
