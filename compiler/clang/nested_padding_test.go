package clang

import "testing"

func TestNestedPaddingInitializer(t *testing.T) {
	const source = `
#pragma pack(push,2)
typedef struct { unsigned :1; signed char child_tag; float child_real; } Child;
#pragma pack(pop)
typedef struct { Child parent_items[2]; int parent_tag; } Parent;
typedef struct { Parent outer_child; int outer_tag; } Outer;
Child echo_child(Child); Parent echo_parent(Parent); Outer echo_outer(Outer);
`
	var producers []string
	for _, target := range targets {
		producers = append(producers, target.triple)
	}
	testCompilerBridgeTargets(t, compilerTool(t), source, nil, producers, "echo_child", "echo_parent", "echo_outer")
}
