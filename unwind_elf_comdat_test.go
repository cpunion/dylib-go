package dylib

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

func comdatFrameObject() *object {
	im := elfUnwindTestImage()
	o := im.objects[0]
	o.sections[2].data = append([]byte(nil), im.mem[128:200]...)
	o.sections = append(o.sections, &section{name: ".text.duplicate", size: 32, exec: true})
	o.symbols = []symbol{{section: 3}, {section: 1}, {section: 2, value: 48}}
	o.relocs = []relocation{{section: 2, offset: 32, symbol: 0}, {section: 2, offset: 56, symbol: 1}}
	o.groups = []sectionGroup{{key: "duplicate", selection: 2, sections: []int{3}}}
	return o
}

func TestELFCOMDATFrameRemapping(t *testing.T) {
	a, b := comdatFrameObject(), comdatFrameObject()
	before := append([]byte(nil), b.sections[2].data...)
	for range 2 {
		out, err := coalesceObjects([]*object{a, b})
		if err != nil {
			t.Fatal(err)
		}
		frame := out[1].sections[2]
		if frame.size != 48 || len(out[1].relocs) != 1 || out[1].relocs[0].offset != 32 || out[1].symbols[2].value != 24 {
			t.Fatalf("frame remapping: %+v, %+v, %+v", frame, out[1].relocs, out[1].symbols)
		}
		if binary.LittleEndian.Uint32(frame.data[28:]) != 28 {
			t.Fatal("retained FDE has a stale CIE back-reference")
		}
		if !bytes.Equal(b.sections[2].data, before) || b.sections[2].size != 72 || len(b.relocs) != 2 || b.symbols[2].value != 48 {
			t.Fatal("link attempt changed staged input")
		}
	}
	for _, test := range []struct {
		name  string
		patch func(*object)
	}{
		{"short_record", func(o *object) { o.sections[2].data = []byte{1} }},
		{"oversized_record", func(o *object) { binary.LittleEndian.PutUint32(o.sections[2].data, 4096) }},
		{"DWARF64", func(o *object) { binary.LittleEndian.PutUint32(o.sections[2].data, 0xffffffff) }},
		{"invalid_cie", func(o *object) { binary.LittleEndian.PutUint32(o.sections[2].data[52:], 0xffffffff) }},
		{"trailing_data", func(o *object) { o.sections[2].data = append(o.sections[2].data, 0, 0, 0, 0, 1) }},
		{"external_reference", func(o *object) { o.relocs = append(o.relocs, relocation{section: 1, symbol: 2}) }},
		{"sized_symbol", func(o *object) { o.symbols[2].size = 8 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, b := comdatFrameObject(), comdatFrameObject()
			test.patch(b)
			data := append([]byte(nil), b.sections[2].data...)
			symbols := append([]symbol(nil), b.symbols...)
			relocs := append([]relocation(nil), b.relocs...)
			if _, err := coalesceObjects([]*object{a, b}); err == nil {
				t.Fatal("invalid frame pruning accepted")
			}
			if !bytes.Equal(data, b.sections[2].data) || !reflect.DeepEqual(symbols, b.symbols) || !reflect.DeepEqual(relocs, b.relocs) {
				t.Fatal("failed pruning changed staged input")
			}
		})
	}
}
