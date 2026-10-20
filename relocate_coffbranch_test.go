package dylib

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"
)

func coffBranchImage(opcode []byte, typ uint32, add int32, executable bool) (*image, *object, relocation, []byte, uintptr) {
	im := &image{base: 0x10000000, mem: make([]byte, 256), stubStart: 128, stubNext: 128, stubs: make(map[uintptr]uintptr), pointerSize: 8}
	sec := &section{offset: 16, size: 32, exec: executable}
	o := &object{info: Info{Format: "COFF", Arch: "amd64"}, sections: []*section{nil, sec}}
	copy(im.mem[sec.offset:], opcode)
	r := relocation{section: 1, offset: uint64(len(opcode)), typ: typ}
	field := sec.offset + r.offset
	b := im.mem[field : field+4]
	le.PutUint32(b, uint32(add))
	return im, o, r, b, im.base + uintptr(field)
}

func TestCOFFAMD64BranchStubs(t *testing.T) {
	opcodes := [][]byte{{0xe8}, {0xe9}}
	for condition := byte(0); condition < 16; condition++ {
		opcodes = append(opcodes, []byte{0x0f, 0x80 | condition})
	}
	for _, opcode := range opcodes {
		for typ := uint32(4); typ <= 9; typ++ {
			for _, offset := range []int32{-16, 0, 16} {
				t.Run(fmt.Sprintf("%x/%d/%d", opcode, typ, offset), func(t *testing.T) {
					add := offset + int32(typ-4)
					im, o, r, b, p := coffBranchImage(opcode, typ, add, true)
					target := uintptr(0xf0000000)
					if err := im.relocCOFF(o, r, b, target, p); err != nil {
						t.Fatal(err)
					}
					destination := int64(p) + 4 + int64(int32(le.Uint32(b)))
					if destination != int64(im.base)+128 {
						t.Fatal("branch does not enter stub", destination)
					}
					if !bytes.Equal(im.mem[128:134], []byte{0xff, 0x25, 0, 0, 0, 0}) || le.Uint64(im.mem[134:142]) != uint64(int64(target)+int64(offset)) {
						t.Fatal("stub lost target offset/bias")
					}
					// The exact absolute destination reuses its existing stub.
					le.PutUint32(b, uint32(add))
					if err := im.relocCOFF(o, r, b, target, p); err != nil || im.stubNext != 144 || len(im.stubs) != 1 {
						t.Fatal("stub reuse", err, im.stubNext)
					}
				})
			}
		}
	}
	for typ := uint32(4); typ <= 9; typ++ {
		t.Run(fmt.Sprintf("near/%d", typ), func(t *testing.T) {
			im, o, r, b, p := coffBranchImage([]byte{0x0f, 0x85}, typ, 7, false)
			target := im.base + 64
			if err := im.relocCOFF(o, r, b, target, p); err != nil {
				t.Fatal(err)
			}
			if got := int64(int32(le.Uint32(b))); got != int64(target)+7-int64(p)-4-int64(typ-4) || len(im.stubs) != 0 {
				t.Fatal("near data relocation changed", got)
			}
		})
	}
}

func TestCOFFAMD64BranchStubValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		opcode []byte
		exec   bool
	}{
		{"data_call", []byte{0xe8}, false}, {"data_conditional", []byte{0x0f, 0x85}, false},
		{"rip_data", []byte{0x48, 0x8d, 0x05}, true}, {"short_branch", []byte{0x75}, true},
		{"other_0f", []byte{0x0f, 0x79}, true}, {"boundary", []byte{0x85}, true}, {"empty", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			im, o, r, b, p := coffBranchImage(tc.opcode, 4, 0, tc.exec)
			im.mem[15] = 0x0f // Cannot borrow an opcode byte from another section.
			before := append([]byte(nil), im.mem...)
			if err := im.relocCOFF(o, r, b, 0xf0000000, p); err == nil || !strings.Contains(err.Error(), "data reference out of range") {
				t.Fatal(err)
			}
			if !bytes.Equal(before, im.mem) || im.stubNext != 128 || len(im.stubs) != 0 {
				t.Fatal("rejected reference changed image")
			}
		})
	}
	for _, negative := range []bool{false, true} {
		t.Run(fmt.Sprint("address/", negative), func(t *testing.T) {
			im, o, r, b, _ := coffBranchImage([]byte{0x0f, 0x85}, 4, 1, true)
			target := ^uintptr(0)
			if uint64(target) > math.MaxUint32 {
				im.base = 0xf0000000
			}
			if negative {
				im.base = 0xf0000000
				target = 0
				le.PutUint32(b, ^uint32(0))
			}
			p := im.base + uintptr(o.sections[1].offset+r.offset)
			if err := im.relocCOFF(o, r, b, target, p); err == nil || !strings.Contains(err.Error(), "address overflow") {
				t.Fatal(err)
			}
			if im.stubNext != 128 {
				t.Fatal("overflow allocated stub")
			}
		})
	}
	im, o, r, b, p := coffBranchImage([]byte{0x0f, 0x85}, 4, 0, true)
	im.mem = im.mem[:136]
	if err := im.relocCOFF(o, r, b, 0xf0000000, p); err == nil {
		t.Fatal("stub capacity ignored")
	}
}
