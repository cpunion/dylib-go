package dylib

import (
	"math"
	"testing"
)

func TestARM64COFFInstructions(t *testing.T) {
	im := &image{base: 0x10000000}
	for _, tc := range []struct {
		name                       string
		typ, instruction, expected uint32
		s, p                       uintptr
	}{
		{"page byte addend carries into next page", 4, 0xb0000000, 0xb0000000, 0x10000fff, 0x10000000},
		{"ADR positive addend", 5, 0x30000000, 0x30000800, 0x10000100, 0x10000000},
		{"ADD page offset", 6, 0x91001400, 0x91009400, 0x10000020, 0x10000000},
		{"ADDS page offset", 6, 0xb1001400, 0xb1009400, 0x10000020, 0x10000000},
		{"LDR scaled page offset", 7, 0xf9400400, 0xf9402400, 0x10000040, 0x10000000},
		{"BL cross section", 3, 0x94000000, 0x94000400, 0x10001000, 0x10000000},
		{"conditional branch", 15, 0x54000000, 0x54000200, 0x10000040, 0x10000000},
		{"test bit branch", 16, 0x36000000, 0x36000200, 0x10000040, 0x10000000},
		{"relative data", 17, 4, 0x100, 0x10000100, 0x10000000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := make([]byte, 4)
			le.PutUint32(b, tc.instruction)
			if err := im.relocCOFFARM64(relocation{typ: tc.typ}, b, tc.s, tc.p); err != nil || le.Uint32(b) != tc.expected {
				t.Fatalf("got %#x, %v; want %#x", le.Uint32(b), err, tc.expected)
			}
		})
	}
	for _, tc := range []struct {
		typ, instruction uint32
		s, p             uintptr
	}{
		{4, 0x10000000, 0x10001000, 0x10000000},  // ADR used for ADRP.
		{5, 0x10000000, 0x10200000, 0x10000000},  // ADR out of range.
		{7, 0xf9400000, 0x10000001, 0x10000000},  // Misaligned 64-bit load.
		{15, 0x54000000, 0x10200000, 0x10000000}, // B.cond out of range.
		{16, 0x36000000, 0x10000001, 0x10000000}, // TBZ misaligned.
		{0xffff, 0, 0x10000000, 0x10000000},
	} {
		b := make([]byte, 4)
		le.PutUint32(b, tc.instruction)
		if err := im.relocCOFFARM64(relocation{typ: tc.typ}, b, tc.s, tc.p); err == nil {
			t.Fatalf("accepted invalid ARM64 relocation %+v", tc)
		}
	}
}

func TestI386RelocationsAndGOT(t *testing.T) {
	im := &image{base: 0x10000000, mem: make([]byte, 64), pointerSize: 4, gotStart: 32, gotNext: 32, stubStart: 48, got: map[uintptr]uintptr{}}
	for _, tc := range []struct {
		typ      uint32
		add      uint32
		implicit bool
		explicit int64
		expected uint32
	}{
		{1, 7, true, 0, 0x10000107},
		{1, 99, false, -4, 0x100000fc},
		{2, 0xfffffffc, true, 0, 0xdc},
		{4, 0xfffffffc, true, 0, 0xdc},
		{9, 4, true, 0, 0xe4},
		{10, 0, true, 0, 0},
		{3, 0, true, 0, 0},
		{43, 0, true, 0, 0},
	} {
		b := make([]byte, 4)
		le.PutUint32(b, tc.add)
		err := im.relocELF386(relocation{typ: tc.typ, implicit: tc.implicit, addend: tc.explicit}, b, 0x10000100, 0x10000020)
		if err != nil || le.Uint32(b) != tc.expected {
			t.Fatalf("relocation %d: got %#x %v; want %#x", tc.typ, le.Uint32(b), err, tc.expected)
		}
	}
	if im.gotNext != 36 || le.Uint32(im.mem[32:]) != 0x10000100 {
		t.Fatal("GOT slot width or reuse incorrect")
	}
	if _, err := im.gotSlot(0x10000200); err != nil || im.gotNext != 40 || le.Uint32(im.mem[36:]) != 0x10000200 {
		t.Fatalf("adjacent 32-bit GOT slot: %v", err)
	}
	b := make([]byte, 4)
	// A displacement crossing 0x80000000 is valid modulo 2^32 on i386.
	if err := im.relocELF386(relocation{typ: 2}, b, 0xf0000000, 0x10000000); err != nil || le.Uint32(b) != 0xe0000000 {
		t.Fatal("PC32 wrap rejected")
	}
	if err := im.relocELF386(relocation{typ: 0xffff}, b, 0, 0); err == nil {
		t.Fatal("unsupported relocation accepted")
	}
	if uint64(^uintptr(0)) > math.MaxUint32 {
		large := uint64(math.MaxUint32) + 1
		if err := im.relocELF386(relocation{typ: 1}, b, uintptr(large), 0); err == nil {
			t.Fatal("64-bit address accepted in ELF32")
		}
	}
}
