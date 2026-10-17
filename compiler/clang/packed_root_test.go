package clang

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func packedRootHeader() (string, []string) {
	var source strings.Builder
	var names []string
	for _, cap := range []int{1, 2, 4, 8, 16} {
		for _, shape := range []struct {
			width int
			real  string
		}{{1, "float"}, {3, "double"}, {31, "float"}, {40, "double"}} {
			for _, mode := range []string{"leading", "interior", "named"} {
				name := fmt.Sprintf("Root_%d_%d_%s", cap, shape.width, mode)
				tag, real := "tag_"+name, "real_"+name
				fields := fmt.Sprintf("unsigned long long :%d; signed char %s; %s %s;", shape.width, tag, shape.real, real)
				if mode == "interior" {
					fields = fmt.Sprintf("signed char %s; unsigned long long :%d; %s %s;", tag, shape.width, shape.real, real)
				} else if mode == "named" {
					fields = "unsigned flags_" + name + ":3; " + fields
				}
				fmt.Fprintf(&source, "#pragma pack(push,%d)\ntypedef struct __attribute__((packed)) {%s} %s;\n#pragma pack(pop)\n%s echo_%s(%s);\n", cap, fields, name, name, name, name)
				names = append(names, "echo_"+name)
			}
		}
	}
	return source.String(), names
}

func TestPackedRootBridge(t *testing.T) {
	source, names := packedRootHeader()
	compiler := compilerTool(t)
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	testCompilerBridgeTargetsWithInspection(t, compiler, source, nil, producers, func(t *testing.T, path string, h *Header) {
		if len(h.Records) != len(names) {
			t.Fatal("missing packed root records", len(h.Records))
		}
		if h.Target.OS != "windows" || h.Target.Arch != "386" {
			testCompilerRecordCallingABI(t, compiler, path, h)
		}
	}, names...)
}

func TestPackedRootGCCBridge(t *testing.T) {
	source, names := packedRootHeader()
	testCompilerRecordGCCBridge(t, compilerTool(t), source, names)
}

func TestPackedRootNativeFixtureBridge(t *testing.T) {
	source, err := os.ReadFile("../../examples/overalignedbits/testdata/exports.h")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"echo_root_byte", "echo_root_leading", "echo_root_named", "echo_root_floating", "echo_root_aligned", "echo_root_envelope"}
	compiler := compilerTool(t)
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	testCompilerBridgeTargetsWithInspection(t, compiler, string(source), nil, producers, func(t *testing.T, path string, h *Header) {
		if len(h.Records) != len(names) {
			t.Fatal("missing native packed roots", len(h.Records))
		}
		if h.Target.OS != "windows" || h.Target.Arch != "386" {
			testCompilerRecordCallingABI(t, compiler, path, h)
		}
	}, names...)
	testCompilerRecordGCCBridge(t, compiler, string(source), names)
}
