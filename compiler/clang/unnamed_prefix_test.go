package clang

import (
	"context"
	"os"
	"strings"
	"testing"
)

func unnamedPrefixFixture(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("../../examples/overalignedbits/testdata/exports.h")
	if err != nil {
		t.Fatal(err)
	}
	return string(source)
}

var unnamedPrefixFunctions = []string{"echo_byte_prefix", "echo_bit_prefix", "echo_float_prefix", "echo_multi_prefix", "echo_typedef_prefix", "echo_prefix_envelope"}

func TestUnnamedPrefixBridge(t *testing.T) {
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	testCompilerBridgeTargetsWithInspection(t, compilerTool(t), unnamedPrefixFixture(t), nil, producers, func(t *testing.T, path string, h *Header) {
		if len(h.Records) != 6 {
			t.Fatal("missing unnamed-prefix records", len(h.Records))
		}
		if h.Target.OS != "windows" || h.Target.Arch != "386" {
			testCompilerRecordCallingABI(t, compilerTool(t), path, h)
		}
	}, unnamedPrefixFunctions...)
}

func TestUnnamedPrefixGCCBridge(t *testing.T) {
	testCompilerRecordGCCBridge(t, compilerTool(t), unnamedPrefixFixture(t), unnamedPrefixFunctions)
}

func TestUnnamedPrefixUnqualified(t *testing.T) {
	for _, fields := range []string{
		"float first; unsigned :3; unsigned :0 __attribute__((aligned(8))); float second;",
		"float first; unsigned :3; unsigned :0 __attribute__((aligned(4))); float second;",
		"signed char tag; unsigned :1; unsigned :0 __attribute__((aligned(8))); float second; unsigned :1;",
		"signed char tag; unsigned :1; unsigned :0 __attribute__((aligned(8))); unsigned :32; float second;",
	} {
		for _, triple := range []string{"x86_64-unknown-linux-gnu", "x86_64-apple-macosx11"} {
			h, err := Parse(context.Background(), headerFile(t, "#pragma pack(push,2)\ntypedef struct {"+fields+"} Value;\n#pragma pack(pop)\nValue echo(Value);"), Options{Compiler: compilerTool(t), Target: triple, Functions: []string{"echo"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = h.CgoSource("bindings", "Bindings"); err == nil || !strings.Contains(err.Error(), "implicit-padding") {
				t.Fatal("unqualified original unnamed values accepted", fields, triple, err)
			}
		}
	}
}
