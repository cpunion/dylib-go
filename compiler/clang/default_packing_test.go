package clang

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const defaultPackingHeader = `typedef struct { signed char plain_tag; double plain_real; int plain_count; } Plain;
typedef struct __attribute__((aligned(32))) { float align_left; float align_right; } Aligned;
typedef struct { signed char member_tag; int member_count __attribute__((aligned(16))); double member_real; } Member;
typedef struct { signed char nested_tag; Plain nested_plain[2]; Aligned nested_align[1]; int nested_count; } Nested;
typedef struct { signed char bits_tag; unsigned bits_low:3; signed bits_high:5; _Bool bits_active:1; unsigned long long bits_wide:40; double bits_real; } Bits;
typedef struct { signed char gaps_tag; unsigned gaps_low:3; unsigned :0; signed gaps_high:7; unsigned long long :0; float gaps_real; } Gaps;
#pragma pack(push,8)
typedef struct { signed char override_tag; double override_real; int override_count; } Override;
#pragma pack(pop)
typedef struct { signed char restored_tag; double restored_real; int restored_count; } Restored;
Plain echo_plain(Plain); Aligned echo_aligned(Aligned); Member echo_member(Member); Nested echo_nested(Nested);
Bits echo_bits(Bits); Gaps echo_gaps(Gaps); Override echo_override(Override); Restored echo_restored(Restored);
`

var defaultPackingFunctions = []string{"echo_plain", "echo_aligned", "echo_member", "echo_nested", "echo_bits", "echo_gaps", "echo_override", "echo_restored"}

func TestCompilerDefaultPackingAcrossTargets(t *testing.T) {
	path, compiler := headerFile(t, defaultPackingHeader), compilerTool(t)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			for _, cap := range []uint64{1, 2, 4, 8, 16} {
				h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Flags: []string{fmt.Sprintf("-fpack-struct=%d", cap)}, Functions: defaultPackingFunctions})
				if err != nil {
					t.Fatal(cap, err)
				}
				plain, _ := h.LookupRecord("Plain")
				override, _ := h.LookupRecord("Override")
				restored, _ := h.LookupRecord("Restored")
				wantOverride := uint64(8)
				if target.os == "windows" && target.arch == "386" {
					wantOverride = cap
				}
				if plain.MaxFieldAlignment != cap || restored.MaxFieldAlignment != cap || override.MaxFieldAlignment != wantOverride {
					t.Fatal("default/override/pop caps", cap, plain, override, restored)
				}
				wantOffset := cap
				if wantOffset > 8 {
					wantOffset = 8
				}
				if target.os == "linux" && target.arch == "386" && wantOffset > 4 {
					wantOffset = 4
				}
				if plain.Layout.Offsets[1] != wantOffset {
					t.Fatal("default storage", cap, plain.Layout)
				}
				generated, err := h.CgoSource("bindings", "Bindings")
				if err != nil || !strings.Contains(string(generated), "MemberAlignments: []uint64{") {
					t.Fatal("default packing persistence", cap, err)
				}
				if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
					t.Fatal("default packing accepted without a C bridge", cap)
				}
			}
		})
	}
}

func TestCompilerDefaultPackingBridgeMSVCAndMinGW(t *testing.T) {
	testBitfieldBridgeMSVCAndMinGW(t, defaultPackingHeader, []string{"-fpack-struct=2"}, defaultPackingFunctions...)
}

func TestCompilerDefaultPackingDriverInputs(t *testing.T) {
	path, compiler := headerFile(t, defaultPackingHeader), compilerTool(t)
	response := filepath.Join(t.TempDir(), "packing flags.rsp")
	if err := os.WriteFile(response, []byte("-fpack-struct=2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		flags []string
		want  uint64
	}{
		{[]string{"-fpack-struct"}, 1},
		{[]string{"@" + response}, 2},
		{[]string{"-Xclang", "-fpack-struct=4"}, 4},
		{[]string{"-fpack-struct=1", "-fpack-struct=8"}, 8},
		{[]string{"-fpack-struct=2", "-Xclang", "-fpack-struct=0"}, 0},
	} {
		h, err := Parse(context.Background(), path, Options{Compiler: compiler, Flags: test.flags, Functions: defaultPackingFunctions})
		if err != nil {
			t.Fatal(test.flags, err)
		}
		r, err := h.LookupRecord("Plain")
		if err != nil || r.MaxFieldAlignment != test.want {
			t.Fatal(test.flags, r, err)
		}
	}
	for _, flags := range [][]string{{"-fpack-struct=32"}, {"-fpack-struct=3"}} {
		if _, err := Parse(context.Background(), path, Options{Compiler: compiler, Flags: flags, Functions: defaultPackingFunctions}); err == nil {
			t.Fatal("unqualified compiler cap accepted", flags)
		}
	}
}

func TestDecodeCompilerDefaultPacking(t *testing.T) {
	for _, test := range []struct {
		command string
		want    uint64
	}{
		{`"clang" "-cc1" "-x" "c"`, 0},
		{`"clang" "-cc1" "-fpack-struct=2"`, 2},
		{`"C:\\Program Files\\clang.exe" "-cc1" "-fpack-struct=4" "-D" "TEXT=\"-fpack-struct=16\""`, 4},
		{`"clang" "-cc1" "-fpack-struct=2" "-fpack-struct=0"`, 0},
	} {
		got, err := decodeDefaultPacking("Clang version\n (in-process)\n " + test.command + "\n")
		if err != nil || got != test.want {
			t.Fatal(test.command, got, err)
		}
	}
	for _, command := range []string{
		"", `"clang" "-cc1as"`, `"clang" "-cc1"` + "\n" + `"clang" "-cc1"`,
		`"clang" "-cc1" "-fpack-struct=32"`, `"clang" "-cc1" "-fpack-struct=3"`,
		`"clang" "-cc1" "-fpack-struct=oops"`, `"clang" "-cc1" "-fpack-struct=-1"`,
		`"clang" "-cc1" "unterminated`, `"clang" "-cc1""missing separator"`,
		`"clang" "-cc1" unquoted`, `"clang" "-cc1" "trailing\`,
	} {
		if _, err := decodeDefaultPacking(command); err == nil {
			t.Fatal("invalid driver metadata accepted", command)
		}
	}
}

func TestCompilerDefaultPackingBeforeBitfieldQualification(t *testing.T) {
	// MSVC i386 ignores pack(8), so default16 permits the explicit request.
	// Qualify the effective cap before applying the over-cap bitfield guard.
	path := headerFile(t, "#pragma pack(push,8)\ntypedef struct {unsigned bits:3 __attribute__((aligned(16)));} S;\n#pragma pack(pop)\nS echo(S);")
	h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: "i686-pc-windows-msvc", Flags: []string{"-fpack-struct=16"}, Functions: []string{"echo"}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := h.LookupRecord("S")
	if err != nil || r.MaxFieldAlignment != 16 || r.Layout.Alignment != 16 {
		t.Fatal("effective bitfield cap", r, err)
	}
	if _, err := h.CgoSource("bindings", "Bindings"); err != nil {
		t.Fatal(err)
	}
	testBitfieldBridgeTargets(t, "#pragma pack(push,8)\ntypedef struct {unsigned bits:3 __attribute__((aligned(16)));} S;\n#pragma pack(pop)\nS echo(S);", []string{"-fpack-struct=16"}, []string{"i686-pc-windows-msvc"}, "echo")
}
