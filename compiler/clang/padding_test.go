package clang

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestUnnamedBitfieldsAcrossTargets(t *testing.T) {
	path := headerFile(t, `typedef struct { unsigned :3; unsigned low:5; unsigned :0; signed high:7; unsigned :4; _Bool enabled:1; unsigned :0; unsigned value; unsigned :5; } Bits;
typedef struct { unsigned :7; unsigned char octet; } Prefix;
typedef Prefix (*Fn)(Prefix);
typedef struct { unsigned :3; Fn entries[2]; unsigned :0; unsigned value; } Entries;
Bits echo(Bits); Prefix prefix(Prefix); Bits *pointer(Bits *); Entries fields(Entries);`)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"echo", "prefix", "pointer", "fields"}})
			if err != nil {
				t.Fatal(err)
			}
			r, err := h.LookupRecord("Bits")
			wantPadding := []BitfieldPadding{{Before: 0, Type: abi.U32, Width: 3}, {Before: 1, Type: abi.U32}, {Before: 2, Type: abi.U32, Width: 4}, {Before: 3, Type: abi.U32}, {Before: 4, Type: abi.U32, Width: 5}}
			wantPadding[1].Offset, wantPadding[3].Offset = 4, 8
			if target.arch == "amd64" && target.os != "windows" {
				wantPadding[0].Mask = []byte{7}
				wantPadding[2].Offset, wantPadding[2].Mask = 4, []byte{128, 7}
				wantPadding[4].Offset, wantPadding[4].Mask = 12, []byte{31}
			}
			if target.os == "windows" {
				wantPadding[3].Offset = 12
			}
			if err != nil || !reflect.DeepEqual(r.Padding, wantPadding) || len(r.Description.Fields) != 4 {
				t.Fatal(r, err)
			}
			size, offsets := uint64(16), []uint64{0, 4, 5, 8}
			if target.os == "windows" {
				size, offsets = 20, []uint64{0, 4, 8, 12}
			}
			if r.Layout.Size != size || !reflect.DeepEqual(r.Layout.Offsets, offsets) || r.Bitfields[0].Mask[0] != 248 {
				t.Fatal("allocation gaps:", r)
			}
			r.Padding[0].Width = 7
			fresh, _ := h.LookupRecord("Bits")
			if !reflect.DeepEqual(fresh.Padding, wantPadding) {
				t.Fatal("padding snapshot is shared")
			}
			p, err := h.LookupRecord("Prefix")
			size, align, offset := uint64(2), uint64(1), uint64(1)
			if target.os == "linux" && target.arch == "arm64" {
				size, align = 4, 4
			} else if target.os == "windows" {
				size, align, offset = 8, 4, 4
			}
			if err != nil || p.Layout.Size != size || p.Layout.Alignment != align || !reflect.DeepEqual(p.Layout.Offsets, []uint64{offset}) || len(p.Bitfields) != 0 || len(p.Padding) != 1 {
				t.Fatal("leading unnamed field:", p, err)
			}
			entries, err := h.LookupRecord("Entries")
			if err != nil || len(entries.FunctionPointers) != 1 || entries.FunctionPointers[0].Position != 0 || len(entries.Description.Fields) != 2 || entries.Description.Fields[0].Type.Len != 2 || entries.FunctionPointers[0].Signature.Result != abi.Struct {
				t.Fatal("function-pointer positions include padding:", entries, err)
			}
			source, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"Padding:", "uint32_t :3;", "uint32_t :0;", "uint32_t :5;", "_record1_transport", "_from_transport("} {
				if !strings.Contains(string(source), marker) {
					t.Fatal("missing unnamed field", marker)
				}
			}
			if target.arch != "386" {
				if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
					t.Fatal("direct padding accepted")
				}
			}
			if h.Target.CheckHost() == nil {
				for _, name := range []string{"echo", "prefix", "pointer", "fields"} {
					if _, err := h.ForHost(name); err == nil || !strings.Contains(err.Error(), "compiled C adapter") {
						t.Fatal(name, err)
					}
				}
			}
		})
	}
}

func TestUnnamedBitfieldValidation(t *testing.T) {
	h, err := Parse(context.Background(), headerFile(t, `typedef struct { unsigned :3; unsigned low:5; unsigned :0; unsigned value; } Bits; Bits echo(Bits);`), Options{Compiler: compilerTool(t), Functions: []string{"echo"}})
	if err != nil {
		t.Fatal(err)
	}
	r := h.Records[0]
	for _, change := range []func(*Record){
		func(r *Record) { r.Padding[0].Before = -1 },
		func(r *Record) { r.Padding[0].Before = 3 },
		func(r *Record) { r.Padding[0].Width = -1 },
		func(r *Record) { r.Padding[0].Width = 33 },
		func(r *Record) { r.Padding[0].Type = abi.F32 },
		func(r *Record) { r.Padding[0].Offset = r.Layout.Size + 1 },
		func(r *Record) { r.Padding[1].Offset = r.Layout.Size + 1 },
		func(r *Record) { r.Padding[0].Before, r.Padding[1].Before = 1, 0 },
		func(r *Record) { r.Padding = make([]BitfieldPadding, 32) },
	} {
		copy := r
		copy.Padding = append([]BitfieldPadding(nil), r.Padding...)
		change(&copy)
		if err := validRecordLayout(copy); err == nil {
			t.Fatal("invalid padding accepted", copy.Padding)
		}
	}
	// Excess native fields must fail even though the logical record has one value.
	path := headerFile(t, "typedef struct {"+strings.Repeat("unsigned :1;", 32)+" unsigned value; } Bits; Bits echo(Bits);")
	if _, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Functions: []string{"echo"}}); err == nil {
		t.Fatal("unnamed fields bypassed record field limit")
	}
}
