package clang

import (
	"context"
	"strings"
	"testing"
)

const unnamedTailHeader = `
typedef struct { float float_value; unsigned :3; } Float;
typedef struct { double double_value; unsigned :3; } Double;
typedef struct { long long integer_value; unsigned :3; } Integer;
typedef struct { float array_values[1]; unsigned :3; } Array;
typedef struct { float first,second,third; unsigned :3; } Triple;
typedef struct { char parent_tag; Float parent_values[2]; } Parent;
Float echo_float(Float); Double echo_double(Double); Integer echo_integer(Integer);
Array echo_array(Array); Triple echo_triple(Triple); Parent echo_parent(Parent);
`

var unnamedTailFunctions = []string{"echo_float", "echo_double", "echo_integer", "echo_array", "echo_triple", "echo_parent"}

func TestUnnamedTailPaddingLayouts(t *testing.T) {
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	testCompilerBridgeTargets(t, compilerTool(t), unnamedTailHeader, nil, producers, unnamedTailFunctions...)
}

func TestUnnamedTailPaddingCallingABI(t *testing.T) {
	path, compiler := headerFile(t, unnamedTailHeader), compilerTool(t)
	for _, triple := range []string{"x86_64-unknown-linux-gnu", "x86_64-apple-macosx11"} {
		t.Run(triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: triple, Functions: unnamedTailFunctions})
			if err != nil {
				t.Fatal(err)
			}
			testCompilerRecordCallingABI(t, compiler, path, h)
		})
	}
}

func TestUnnamedTailPaddingGCCBridge(t *testing.T) {
	testCompilerRecordGCCBridge(t, compilerTool(t), unnamedTailHeader, unnamedTailFunctions)
}

func TestUnnamedTailPaddingUnqualified(t *testing.T) {
	compiler := compilerTool(t)
	for _, source := range []string{
		`typedef struct { float values[2]; unsigned :3; } Pair; Pair f(Pair);`,
		`typedef struct { unsigned :3; float values[2]; unsigned :3; } Pair; Pair f(Pair);`,
	} {
		for _, triple := range []string{"x86_64-unknown-linux-gnu", "x86_64-apple-macosx11"} {
			h, err := Parse(context.Background(), headerFile(t, source), Options{Compiler: compiler, Target: triple, Functions: []string{"f"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.CgoSource("bindings", "Bindings"); err == nil || !strings.Contains(err.Error(), "implicit-padding") {
				t.Fatal("unqualified trailing padding accepted", err)
			}
		}
	}
}
