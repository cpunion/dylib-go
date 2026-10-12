package clang

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestBitfieldRecordsAcrossTargets(t *testing.T) {
	path := headerFile(t, `typedef struct { unsigned flags:3; signed delta:5; _Bool active:1; unsigned value; } Bits;
typedef struct { Bits items[2]; } Outer;
Bits echo(Bits); Outer nest(Outer); Bits *pointer(Bits *);`)
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Target: target.triple, Functions: []string{"echo", "nest", "pointer"}})
			if err != nil {
				t.Fatal(err)
			}
			r, err := h.LookupRecord("Bits")
			if err != nil || len(r.Bitfields) != 3 {
				t.Fatal(r, err)
			}
			active, value, size := uint64(1), uint64(4), uint64(8)
			if target.os == "windows" {
				active, value, size = 4, 8, 12
			}
			if r.Layout.Size != size || !reflect.DeepEqual(r.Layout.Offsets, []uint64{0, 0, active, value}) || !reflect.DeepEqual(r.Bitfields, []Bitfield{{Position: 0, Width: 3, Mask: []byte{7}}, {Position: 1, Width: 5, Mask: []byte{248}}, {Position: 2, Width: 1, Mask: []byte{1}}}) {
				t.Fatal("compiler bit masks:", r)
			}
			r.Bitfields[0].Mask[0] = 255
			fresh, _ := h.LookupRecord("Bits")
			if fresh.Bitfields[0].Mask[0] != 7 {
				t.Fatal("mask snapshot is shared")
			}
			source, err := h.CgoSource("bindings", "Bindings")
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"Bitfields:", "uint32_t f0:3;", "int32_t f1:5;", "_Bool f2:1;", "_mask(0,UINT64_MAX)", "_from_transport(p0)"} {
				if !strings.Contains(string(source), marker) {
					t.Fatal("missing generated bitfield", marker)
				}
			}
			if target.arch != "386" {
				if _, err := h.LLGoSource("bindings", "Bindings"); err == nil {
					t.Fatal("direct bitfields accepted")
				}
			}
			if h.Target.CheckHost() == nil {
				for _, name := range []string{"echo", "nest", "pointer"} {
					if _, err := h.ForHost(name); err == nil || !strings.Contains(err.Error(), "compiled C adapter") {
						t.Fatal(name, err)
					}
				}
			}
		})
	}
}

func TestBitfieldMaskValidation(t *testing.T) {
	h, err := Parse(context.Background(), headerFile(t, `typedef struct { unsigned flags:3; signed delta:5; } Bits; Bits echo(Bits);`), Options{Compiler: compilerTool(t), Functions: []string{"echo"}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := h.LookupRecord("Bits")
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []func(*Record){
		func(r *Record) { r.Bitfields[0].Mask = nil },
		func(r *Record) { r.Bitfields[0].Mask = []byte{11} },
		func(r *Record) { r.Bitfields[0].Width = 65 },
		func(r *Record) { r.Bitfields[0].Position = 20 },
		func(r *Record) { r.Bitfields[1].Position = -1 },
		func(r *Record) { r.Bitfields[1].Position = 0 },
		func(r *Record) { r.Bitfields[1].Mask = []byte{31} },
		func(r *Record) { r.Bitfields[0].Mask = make([]byte, 10) },
		func(r *Record) { r.Layout.Offsets[1] = r.Layout.Size },
	} {
		copy := r
		copy.Bitfields = cloneBitfields(r.Bitfields)
		copy.Layout.Offsets = append([]uint64(nil), r.Layout.Offsets...)
		mutation(&copy)
		if err := validRecordLayout(copy); err == nil {
			t.Fatal("invalid bitfield accepted", copy)
		}
	}
	for _, os := range []string{"linux", "darwin", "windows", "other"} {
		if _, err := readBitfieldSymbols([]byte("broken"), os); err == nil {
			t.Fatal("invalid probe accepted", os)
		}
	}
	for _, name := range []string{"missing", "short"} {
		if _, err := (bitfieldSymbols{"short": {1}}).value(name, 8); err == nil {
			t.Fatal("invalid symbol accepted", name)
		}
	}
}
