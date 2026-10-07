package dylib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeCOMDATAndSharedInlineState(t *testing.T) {
	needNative(t)
	dir := t.TempDir()
	a := compile(t, "testdata/comdat_a.cpp", filepath.Join(dir, "a.o"), "-fno-exceptions", "-fno-rtti", "-std=c++17")
	b := compile(t, "testdata/comdat_b.cpp", filepath.Join(dir, "b.o"), "-fno-exceptions", "-fno-rtti", "-std=c++17")
	archive := filepath.Join(dir, "inline.a")
	command(t, "ar", "rcs", archive, a, b)
	for _, inputs := range [][]string{{a, b}, {b, a}, {archive}} {
		s := New(Options{})
		load(t, s, inputs...)
		if err := s.Link("from_a", "from_b"); err != nil {
			s.Close()
			t.Fatal(err)
		}
		call(t, s, "from_a", 20, 22, 43)
		call(t, s, "from_b", 20, 22, 44)
		call(t, s, "from_a", 20, 22, 45)
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// This assembler fixture has a real relocation to a missing symbol only in
// the second definition. Discarding its COMDAT must also discard the dependency.
func TestForeignCOMDATDiscardedReferences(t *testing.T) {
	for _, tc := range []struct{ target, good, bad string }{
		{"x86_64-linux-gnu", ".section .data.shared,\"awG\",@progbits,shared,comdat\n.globl shared\nshared:\n.quad 7\n", ".section .data.shared,\"awG\",@progbits,shared,comdat\n.globl shared\nshared:\n.quad missing_only_in_discarded\n"},
		{"aarch64-linux-gnu", ".section .data.shared,\"awG\",@progbits,shared,comdat\n.globl shared\nshared:\n.quad 7\n", ".section .data.shared,\"awG\",@progbits,shared,comdat\n.globl shared\nshared:\n.quad missing_only_in_discarded\n"},
		{"i686-linux-gnu", ".section .data.shared,\"awG\",@progbits,shared,comdat\n.globl shared\nshared:\n.long 7\n", ".section .data.shared,\"awG\",@progbits,shared,comdat\n.globl shared\nshared:\n.long missing_only_in_discarded\n"},
		{"x86_64-pc-windows-msvc", ".section .data$shared,\"dw\",discard,shared\n.globl shared\nshared:\n.quad 7\n", ".section .data$shared,\"dw\",discard,shared\n.globl shared\nshared:\n.quad missing_only_in_discarded\n"},
		{"aarch64-pc-windows-msvc", ".section .data$shared,\"dw\",discard,shared\n.globl shared\nshared:\n.quad 7\n", ".section .data$shared,\"dw\",discard,shared\n.globl shared\nshared:\n.quad missing_only_in_discarded\n"},
		{"i686-pc-windows-msvc", ".section .data$shared,\"dw\",discard,shared\n.globl shared\nshared:\n.long 7\n", ".section .data$shared,\"dw\",discard,shared\n.globl shared\nshared:\n.long missing_only_in_discarded\n"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			dir := t.TempDir()
			var objects []*object
			for i, source := range []string{tc.good, tc.bad} {
				file := filepath.Join(dir, []string{"good.s", "bad.s"}[i])
				if err := os.WriteFile(file, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				out := file + ".o"
				command(t, compiler(), "--target="+tc.target, "-c", file, "-o", out)
				data, err := readFile(out)
				if err != nil {
					t.Fatal(err)
				}
				parsed, err := parse(out, data)
				if err != nil {
					t.Fatal(err)
				}
				if len(parsed.info.Unsupported) != 0 || len(parsed.obj.groups) == 0 {
					t.Fatalf("COMDAT metadata: %+v", parsed.info)
				}
				objects = append(objects, parsed.obj)
			}
			coalesced, err := coalesceObjects(objects)
			if err != nil {
				t.Fatal(err)
			}
			if len(objects[1].relocs) != 1 || len(coalesced[1].relocs) != 0 {
				t.Fatal("discard or input snapshot failed")
			}
			defs, err := definitions(coalesced)
			if err != nil || defs["shared"].o != coalesced[0] {
				t.Fatalf("wrong COMDAT selection: %v", err)
			}
			s := New(Options{})
			s.files = []*file{{obj: objects[0]}, {obj: objects[1]}}
			selected, err := s.selectObjects([]string{"shared"})
			if err != nil || len(selected[1].relocs) != 0 {
				t.Fatalf("discarded dependency survived: %v", err)
			}
		})
	}
}

func TestCOMDATSelectionAndAssociations(t *testing.T) {
	makeObject := func(selection uint8, size uint64) *object {
		return &object{info: Info{Format: "COFF"}, sections: []*section{nil, {size: size}, {size: 4}},
			groups:  []sectionGroup{{key: "shared", sections: []int{1}, selection: selection}, {sections: []int{2}, selection: 5, parent: 1}},
			symbols: []symbol{{name: "shared", section: 1, global: true}, {name: "associated", section: 2, global: true}}}
	}
	for _, selection := range []uint8{1, 2, 3, 6} {
		a, b := makeObject(selection, 4), makeObject(selection, 8)
		out, err := coalesceObjects([]*object{a, b})
		if selection == 1 || selection == 3 {
			if err == nil {
				t.Fatal("invalid duplicate accepted")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		winner := 0
		if selection == 6 {
			winner = 1
		}
		if out[winner].sections[1] == nil || out[winner].sections[2] == nil || out[1-winner].sections[1] != nil || out[1-winner].sections[2] != nil {
			t.Fatal("association was not retained/discarded with its parent")
		}
	}
	a, b := makeObject(3, 4), makeObject(3, 4)
	if _, err := coalesceObjects([]*object{a, b}); err != nil {
		t.Fatal(err)
	}
	b.groups[0].selection = 2
	if _, err := coalesceObjects([]*object{a, b}); err == nil {
		t.Fatal("conflicting rules accepted")
	}
	a.groups[0] = sectionGroup{sections: []int{1}, selection: 5, parent: 2}
	if _, err := coalesceObjects([]*object{a}); err == nil {
		t.Fatal("association cycle accepted")
	}
}
