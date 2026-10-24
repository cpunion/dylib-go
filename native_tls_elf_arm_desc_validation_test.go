package dylib

import (
	"bytes"
	"fmt"
	"testing"
)

func armTLSDescInstructions() []struct {
	name     string
	typ, ins uint32
} {
	return []struct {
		name     string
		typ, ins uint32
	}{
		{"literal", 560, 0x58000003}, {"adr", 561, 0x10000003},
		{"page", 562, 0x90000003}, {"load_page", 563, 0xf9400043},
		{"add_page", 564, 0x91000043}, {"movw_g1", 565, 0xd2a00003},
		{"movw_g0", 566, 0xf2800003},
		{"load_immediate", 567, 0xf9400043}, {"load_register", 567, 0xf8606843},
		{"add_immediate", 568, 0x91000043}, {"add_register", 568, 0x8b000043},
		{"call", 569, 0xd63f0060},
	}
}

func TestELFARMTLSDescFormValidation(t *testing.T) {
	for _, form := range armTLSDescInstructions() {
		cases := []string{"not_enabled", "noninstruction", "addend", "opcode", "encoded_operand", "misaligned", "foreign", "non_tls", "missing_record", "bounds", "width"}
		if form.typ == 565 || form.typ == 566 {
			cases = append(cases, "shift")
		}
		for _, name := range cases {
			t.Run(form.name+"/"+name, func(t *testing.T) {
				im, o := elfTLSValidationImage()
				o.info.Arch = "arm64"
				r := relocation{section: 1, symbol: 1, typ: form.typ}
				o.relocs = []relocation{r}
				if _, err := im.layoutELFTLS(128); err != nil {
					t.Fatal(err)
				}
				le.PutUint32(im.mem, form.ins)
				if err := im.relocate(o, r); err != nil {
					t.Fatal("valid descriptor form:", err)
				}
				le.PutUint32(im.mem, form.ins)
				switch name {
				case "not_enabled":
					im.elfTLS = nil
				case "noninstruction":
					o.sections[1].exec = false
				case "addend":
					r.addend = 1
				case "opcode":
					le.PutUint32(im.mem, 0)
				case "encoded_operand":
					field := uint(5)
					if form.typ == 563 || form.typ == 564 || form.typ >= 567 {
						field = 10
					}
					if form.name == "load_register" {
						field = 12 // LSL #3 is outside the qualified register form.
					}
					le.PutUint32(im.mem, form.ins|1<<field)
				case "misaligned":
					r.offset = 1
				case "foreign":
					o.symbols[1].section = 0
				case "non_tls":
					o.symbols[1].tls = false
				case "missing_record":
					im.elfTLS.records = nil
				case "bounds":
					r.offset = o.sections[1].size - 1
				case "width":
					le.PutUint32(im.mem, form.ins^1<<31)
				case "shift":
					le.PutUint32(im.mem, form.ins^1<<21)
				}
				if err := im.relocate(o, r); err == nil {
					t.Fatal("invalid descriptor form accepted")
				}
			})
		}
	}
}

func TestELFARMTLSDescAddressFields(t *testing.T) {
	for _, tc := range []struct {
		typ, ins, want uint32
	}{
		{560, 0x58000003, 0x58001203}, // LDR x3, descriptor at P+0x240.
		{561, 0x10000003, 0x10001203}, // ADR x3, the same descriptor.
		{565, 0xd2a00003, 0x92a00003}, // descriptor-GOT = -0xdc0; MOVN G1.
		{566, 0xf2800003, 0xf29e4803}, // low 16 bits are 0xf240.
	} {
		t.Run(fmt.Sprint(tc.typ), func(t *testing.T) {
			im, o := elfTLSValidationImage()
			o.info.Arch = "arm64"
			im.gotStart = 0x1000
			im.elfTLS = &elfTLSLayout{records: map[definition]uint64{{o, 1}: 0x230}}
			le.PutUint32(im.mem, tc.ins)
			if err := im.relocate(o, relocation{section: 1, symbol: 1, typ: tc.typ}); err != nil || le.Uint32(im.mem) != tc.want {
				t.Fatalf("descriptor half/address field: %#x, %v; want %#x", le.Uint32(im.mem), err, tc.want)
			}
		})
	}
}

func TestELFARMTLSDescAnnotationsPreserveInstructions(t *testing.T) {
	for _, form := range armTLSDescInstructions() {
		if form.typ < 567 {
			continue
		}
		t.Run(form.name, func(t *testing.T) {
			im, o := elfTLSValidationImage()
			o.info.Arch = "arm64"
			im.elfTLS = &elfTLSLayout{records: map[definition]uint64{{o, 1}: 128}}
			le.PutUint32(im.mem, form.ins)
			want := append([]byte(nil), im.mem...)
			if err := im.relocate(o, relocation{section: 1, symbol: 1, typ: form.typ}); err != nil || !bytes.Equal(im.mem, want) {
				t.Fatal("relaxation annotation changed image bytes:", err)
			}
		})
	}
}

func TestELFARMTLSDescPCRelativeRanges(t *testing.T) {
	for _, typ := range []uint32{560, 561} {
		for _, offset := range []int64{-(1 << 20) - 4, -(1 << 20), -4, 0, 1, 4, (1 << 20) - 4, (1 << 20) - 1, 1 << 20} {
			t.Run(fmt.Sprint(typ, "/", offset), func(t *testing.T) {
				im, o := elfTLSValidationImage()
				o.info.Arch = "arm64"
				im.base = 1 << 24
				place := im.base + 2<<20
				im.elfTLS = &elfTLSLayout{records: map[definition]uint64{{o, 1}: uint64((2<<20)+offset) - 16}}
				ins := uint32(0x10000003)
				if typ == 560 {
					ins = 0x58000003
				}
				le.PutUint32(im.mem, ins)
				_, err := im.relocateELFTLS(o, relocation{section: 1, symbol: 1, typ: typ}, im.mem[:4], place)
				valid := offset >= -(1<<20) && offset < 1<<20 && (typ == 561 || offset%4 == 0)
				if !valid {
					if err == nil || le.Uint32(im.mem) != ins {
						t.Fatal("PC-relative overflow/misalignment accepted or modified instruction:", err)
					}
					return
				}
				got := le.Uint32(im.mem)
				delta := signExtend((got>>29&3)|(got>>5&0x7ffff)<<2, 21)
				if typ == 560 {
					delta = signExtend(got>>5&0x7ffff, 19) << 2
				}
				if err != nil || delta != offset || got&31 != 3 {
					t.Fatal("PC-relative descriptor field/operand:", delta, offset, err)
				}
			})
		}
	}
}
