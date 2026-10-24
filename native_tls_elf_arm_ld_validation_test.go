package dylib

import (
	"fmt"
	"testing"
)

func armTLSLDInstructions() map[uint32]uint32 {
	return map[uint32]uint32{
		517: 0x10000000, 518: 0x90000000, 519: 0x91000000,
		520: 0xd2a00000, 521: 0xf2800000, 522: 0x9c000000,
		523: 0xd2c00000, 524: 0xd2a00000, 525: 0xf2a00000,
		526: 0xd2800000, 527: 0xf2800000,
		528: 0x91400000, 529: 0x91000000, 530: 0x91000000,
		531: 0x39400000, 532: 0x39000000,
		533: 0x79400000, 534: 0x79000000,
		535: 0xb9400000, 536: 0xb9000000,
		537: 0xf9400000, 538: 0xf9000000,
		572: 0x3dc00000, 573: 0x3d800000,
	}
}

func TestELFARMLocalDynamicTLSValidation(t *testing.T) {
	for typ, ins := range armTLSLDInstructions() {
		cases := []string{"not_enabled", "noninstruction", "addend", "opcode", "encoded_immediate", "misaligned", "foreign", "non_tls", "bounds"}
		if typ <= 522 {
			cases = append(cases, "missing_module_record")
		} else {
			cases = append(cases, "negative_offset", "past_template_end")
		}
		if typ == 520 || typ == 521 || typ >= 523 && typ <= 527 {
			cases = append(cases, "width", "shift")
		}
		for _, name := range cases {
			t.Run(fmt.Sprint(typ, "/", name), func(t *testing.T) {
				im, o := elfTLSValidationImage()
				o.info.Arch = "arm64"
				o.symbols[1].value, o.sections[2].size = 16, 128
				r := relocation{section: 1, symbol: 1, typ: typ}
				o.relocs = []relocation{r}
				if _, err := im.layoutELFTLS(256); err != nil {
					t.Fatal(err)
				}
				le.PutUint32(im.mem, ins)
				if err := im.relocate(o, r); err != nil {
					t.Fatal("valid LD instruction:", err)
				}
				le.PutUint32(im.mem, ins)
				switch name {
				case "not_enabled":
					im.elfTLS = nil
				case "noninstruction":
					o.sections[1].exec = false
				case "addend":
					r.addend = 1
					if typ >= 523 {
						r.addend = 128 // DTPREL allows valid nonzero addends.
					}
				case "opcode":
					le.PutUint32(im.mem, 0)
				case "width":
					le.PutUint32(im.mem, ins^1<<31)
				case "shift":
					le.PutUint32(im.mem, ins^1<<21)
				case "encoded_immediate":
					field := uint(5)
					if typ >= 528 || typ == 519 {
						field = 10
					}
					le.PutUint32(im.mem, ins|1<<field)
				case "misaligned":
					r.offset = 1
				case "foreign":
					o.symbols[1].section = 0
				case "non_tls":
					o.symbols[1].tls = false
				case "bounds":
					r.offset = o.sections[1].size - 1
				case "missing_module_record":
					im.elfTLS.records = nil
				case "negative_offset":
					r.addend = -17
				case "past_template_end":
					r.addend = 113
				}
				if err := im.relocate(o, r); err == nil {
					t.Fatal("invalid local-dynamic relocation accepted")
				}
			})
		}
	}
}

func TestELFARMDTPRELFields(t *testing.T) {
	for _, tc := range []struct {
		typ, ins, want uint32
		value          uint64
	}{
		{523, 0x92c00003, 0xd2c24683, 0x123456789abc},
		{524, 0x92a00003, 0xd2a24683, 0x12345678},
		{525, 0xf2a00003, 0xf2aacf03, 0x123456789abc},
		{526, 0x92800003, 0xd29579a3, 0xabcd},
		{527, 0xf2800003, 0xf2935783, 0x123456789abc},
		{528, 0x91400023, 0x91448c23, 0x123456},
		{529, 0x91000023, 0x910d1423, 0x345},
		{530, 0x91000023, 0x910d1423, 0x12345},
		{531, 0x39400023, 0x394fc023, 0x3f0},
		{532, 0x39000023, 0x390fc023, 0x123f0},
		{533, 0x79400023, 0x7947e023, 0x3f0},
		{534, 0x79000023, 0x7907e023, 0x123f0},
		{535, 0xbd400023, 0xbd43f023, 0x3f0},
		{536, 0xbd000023, 0xbd03f023, 0x123f0},
		{537, 0xfd400023, 0xfd41f823, 0x3f0},
		{538, 0xfd000023, 0xfd01f823, 0x123f0},
		{572, 0x3dc00023, 0x3dc0fc23, 0x3f0},
		{573, 0x3d800023, 0x3d80fc23, 0x123f0},
	} {
		t.Run(fmt.Sprint(tc.typ), func(t *testing.T) {
			b := make([]byte, 4)
			le.PutUint32(b, tc.ins)
			if err := armELFTLSOffset(b, tc.typ, tc.value, 16, 0); err != nil || le.Uint32(b) != tc.want {
				t.Fatalf("DTPREL field: %#x, %v; want %#x", le.Uint32(b), err, tc.want)
			}
		})
	}
}

func TestELFARMDTPRELRanges(t *testing.T) {
	for _, tc := range []struct {
		typ  uint32
		bits uint
	}{
		{523, 48}, {524, 32}, {526, 16}, {528, 24}, {529, 12},
		{531, 12}, {533, 12}, {535, 12}, {537, 12}, {572, 12},
	} {
		for _, edge := range []string{"last", "overflow"} {
			t.Run(fmt.Sprint(tc.typ, "/", edge), func(t *testing.T) {
				b := make([]byte, 4)
				ins := armTLSLDInstructions()[tc.typ]
				le.PutUint32(b, ins)
				value := uint64(1) << tc.bits
				if edge == "last" {
					value--
					if tc.typ >= 531 {
						scale, _ := armOffsetScale(ins)
						value &^= (1 << scale) - 1
					}
				}
				err := armELFTLSOffset(b, tc.typ, value, 16, 0)
				if edge == "last" && err != nil || edge == "overflow" && (err == nil || le.Uint32(b) != ins) {
					t.Fatal("DTPREL range accepted/rejected incorrectly or damaged instruction:", err)
				}
			})
		}
	}
	for _, typ := range []uint32{531, 532, 533, 534, 535, 536, 537, 538, 572, 573} {
		ins := armTLSLDInstructions()[typ]
		scale, _ := armOffsetScale(ins)
		cases := []string{"extent", "wrong_width", "signed_or_alternate"}
		if scale != 0 {
			cases = append(cases, "alignment")
		}
		for _, name := range cases {
			t.Run(fmt.Sprint(typ, "/", name), func(t *testing.T) {
				b := make([]byte, 4)
				le.PutUint32(b, ins)
				value, remaining := uint64(16), uint64(16)
				switch name {
				case "extent":
					remaining = (1 << scale) - 1
				case "alignment":
					value++
				case "wrong_width":
					le.PutUint32(b, ins^1<<30)
				case "signed_or_alternate":
					le.PutUint32(b, ins^1<<23)
				}
				if err := armELFTLSOffset(b, typ, value, remaining, 0); err == nil {
					t.Fatal("invalid DTPREL datum access accepted")
				}
			})
		}
	}
}

func TestELFARMDTPRELPackedOffsetsAndAddends(t *testing.T) {
	for _, addend := range []int64{-8, 0, 8, 48, 56} {
		t.Run(fmt.Sprint(addend), func(t *testing.T) {
			im, o := elfTLSValidationImage()
			o.info.Arch = "arm64"
			im.elfTLS = &elfTLSLayout{sections: map[*section]uint64{o.sections[2]: 0x1230}}
			le.PutUint32(im.mem, 0x91000023) // add x3, x1, #offset
			r := relocation{section: 1, symbol: 1, typ: 530, addend: addend}
			if err := im.relocate(o, r); err != nil {
				t.Fatal(err)
			}
			ins := le.Uint32(im.mem)
			want := uint32(0x1238+addend) & 4095
			if ins>>10&4095 != want || ins&1023 != 0x23 {
				t.Fatalf("DTPREL did not include the packed section offset and addend: %#x", ins)
			}
		})
	}
}
