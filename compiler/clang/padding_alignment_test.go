package clang

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

const paddingAlignmentHeader = `
typedef struct { char lead_marker; unsigned :3 __attribute__((aligned(16))); unsigned lead_value:5; char lead_tail; } Leading;
typedef struct { char shared_marker; unsigned shared_value:3; unsigned :5 __attribute__((aligned(16))); char shared_tail; } Shared;
typedef struct __attribute__((packed)) { char global_marker; unsigned :3 __attribute__((aligned(16))); unsigned global_value:5; char global_tail; } PackedLeading;
typedef struct { char member_marker; unsigned :3 __attribute__((packed,aligned(16))); unsigned member_value:5; char member_tail; } MemberLeading;
typedef struct { char active_marker; unsigned active_value:3 __attribute__((packed)); unsigned :0 __attribute__((aligned(16))); char active_tail; } Active;
typedef struct { float ignored_left; unsigned :0 __attribute__((aligned(16))); float ignored_right; } Ignored;
typedef struct { char consecutive_marker; unsigned consecutive_value:3 __attribute__((packed)); unsigned :0 __attribute__((aligned(8))); unsigned :0 __attribute__((aligned(32))); char consecutive_tail; } Consecutive;
typedef struct { unsigned trailing_value:3 __attribute__((packed)); unsigned :0 __attribute__((packed,aligned(2))); } Trailing;
typedef struct __attribute__((packed)) { char packed_marker; unsigned packed_value:3; unsigned :0 __attribute__((aligned(16))); char packed_tail; } PackedActive;
typedef struct { char overflow_marker; unsigned :31 __attribute__((packed,aligned(8))); unsigned :2 __attribute__((packed,aligned(16))); unsigned overflow_value:7; char overflow_tail; } Overflow;
typedef struct { char only_marker; unsigned :3 __attribute__((packed,aligned(16))); } PaddingOnly;
Leading leading(Leading); Shared shared(Shared); PackedLeading packed_leading(PackedLeading); MemberLeading member_leading(MemberLeading); Active active(Active); Ignored ignored(Ignored); Consecutive consecutive(Consecutive); Trailing trailing(Trailing); PackedActive packed_active(PackedActive); Overflow overflow(Overflow); PaddingOnly padding_only(PaddingOnly);
`

var paddingAlignmentFunctions = []string{"leading", "shared", "packed_leading", "member_leading", "active", "ignored", "consecutive", "trailing", "packed_active", "overflow", "padding_only"}

func TestUnnamedAlignmentAcrossTargets(t *testing.T) {
	path, compiler := headerFile(t, paddingAlignmentHeader), compilerTool(t)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: paddingAlignmentFunctions})
			if err != nil {
				t.Fatal(err)
			}
			leading, err := h.LookupRecord("Leading")
			if err != nil || leading.Padding[0].Aligned != 16 || leading.Padding[0].Offset != 16 {
				t.Fatal("aligned unnamed position:", leading, err)
			}
			leading.Padding[0].Aligned = 32
			fresh, _ := h.LookupRecord("Leading")
			if fresh.Padding[0].Aligned != 16 {
				t.Fatal("padding alignment snapshot aliases header")
			}
			if target.os == "windows" {
				shared, _ := h.LookupRecord("Shared")
				ignored, _ := h.LookupRecord("Ignored")
				consecutive, _ := h.LookupRecord("Consecutive")
				if shared.Padding[0].Offset != 4 || shared.Layout.Alignment != 4 || shared.Layout.Size != 12 || ignored.Layout.Size != 8 || ignored.Layout.Alignment != 4 || ignored.Padding[0].Offset != 4 || consecutive.Padding[1].Offset != 8 || consecutive.Layout.Alignment != 8 {
					t.Fatal("MSVC ignored alignment rules:", shared, ignored, consecutive)
				}
			}
			// Omitted GNU arguments use the compiler's maximum alignment, not
			// the original record's possibly smaller effective alignment.
			omitted := strings.ReplaceAll(paddingAlignmentHeader, "aligned(16)", "aligned")
			other, err := Parse(context.Background(), headerFile(t, omitted), Options{Compiler: compiler, Target: target.triple, Functions: paddingAlignmentFunctions})
			if err != nil || !reflect.DeepEqual(h.Records, other.Records) {
				t.Fatal("omitted unnamed alignment:", other, err)
			}
			if _, err := h.CgoSource("bindings", "Bindings"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUnnamedAlignmentBridgeMSVCAndMinGW(t *testing.T) {
	testPackedBitfieldBridgeMSVCAndMinGW(t, paddingAlignmentHeader, paddingAlignmentFunctions...)
}

func TestInvalidUnnamedAlignmentMetadata(t *testing.T) {
	h, err := Parse(context.Background(), headerFile(t, paddingAlignmentHeader), Options{Compiler: compilerTool(t), Functions: []string{"leading"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, alignment := range []uint64{3, 131072} {
		r := h.Records[0]
		r.Padding = append([]BitfieldPadding(nil), r.Padding...)
		r.Padding[0].Aligned = alignment
		if err := validRecordLayout(r); err == nil {
			t.Fatal("invalid unnamed alignment accepted", alignment)
		}
	}
	r := h.Records[0]
	r.Padding = append([]BitfieldPadding(nil), r.Padding...)
	r.Padding[0].Offset = r.Layout.Size
	if err := validRecordLayout(r); err == nil {
		t.Fatal("aligned nonzero padding outside record accepted")
	}
}
