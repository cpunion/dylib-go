package clang

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Compare physical LLVM parameters/results while ignoring local record names.
func testCompilerRecordCallingABI(t *testing.T, compiler, path string, h *Header) {
	t.Helper()
	generated, err := h.CgoSource("bindings", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	var original, bridge strings.Builder
	fmt.Fprintf(&original, "#include %q\n", filepath.ToSlash(path))
	bridge.WriteString(strings.Split(strings.Split(text, "/*")[1], "*/")[0])
	for i, record := range h.Records {
		fmt.Fprintf(&original, "%s call_%d(%s v){return v;}\n", record.Name, i, record.Name)
		fmt.Fprintf(&bridge, "dylib_go_%x_record%d call_%d(dylib_go_%x_record%d v){return v;}\n", "Bindings", i, i, "Bindings", i)
	}
	compile := func(source string) string {
		t.Helper()
		args := []string{"--target=" + h.Target.Triple, "-ffreestanding", "-x", "c", "-std=c17", "-O0", "-S", "-emit-llvm", "-o", "-", "-"}
		ir, err := runCompiler(context.Background(), compiler, args, source, maxASTSize)
		if err != nil {
			t.Fatal(err)
		}
		return string(ir)
	}
	nativeIR, bridgeIR := compile(original.String()), compile(bridge.String())
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
		want, got := entry(nativeIR, name), entry(bridgeIR, name)
		if want == "" || got != want {
			t.Fatalf("%s ABI: got %s; want %s", record.Name, got, want)
		}
	}
}
