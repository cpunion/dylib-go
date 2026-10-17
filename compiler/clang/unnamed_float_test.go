package clang

import (
	"context"
	"strings"
	"testing"
)

func TestUnnamedFloatPaddingRequiresFacade(t *testing.T) {
	for _, source := range []string{
		"#pragma pack(push,4)\ntypedef struct __attribute__((packed)) {unsigned :32; float real;} Value;\n#pragma pack(pop)",
		"typedef struct {unsigned :32; float real;} Value;",
		"typedef struct {float first; unsigned :3; float second;} Value;",
		"typedef struct {float inner;} Inner; typedef struct {unsigned :32; Inner child;} Value;",
		"typedef struct {unsigned :32; float items[1];} Value;",
		"typedef struct {unsigned long long :64; double real;} Value;",
	} {
		for _, triple := range []string{"x86_64-unknown-linux-gnu", "x86_64-apple-macosx11"} {
			h, err := Parse(context.Background(), headerFile(t, source+"\nValue echo(Value);"), Options{Compiler: compilerTool(t), Target: triple, Functions: []string{"echo"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = h.CgoSource("bindings", "Bindings"); err == nil || !strings.Contains(err.Error(), "implicit-padding C bridge") {
				t.Fatal("unqualified floating register classes accepted", source, triple, err)
			}
		}
	}
}

func TestUnnamedFloatPaddingMemoryBridge(t *testing.T) {
	const source = `
typedef struct __attribute__((packed)) {unsigned :1; float first;} Leading;
typedef struct __attribute__((packed)) {float left; unsigned :3; float right;} Interior;
typedef struct __attribute__((packed)) {unsigned :1; float items[2];} Array;
Leading echo_leading(Leading); Interior echo_interior(Interior); Array echo_array(Array);
`
	names := []string{"echo_leading", "echo_interior", "echo_array"}
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	testCompilerBridgeTargets(t, compilerTool(t), source, nil, producers, names...)
	testCompilerRecordGCCBridge(t, compilerTool(t), source, names)
}
