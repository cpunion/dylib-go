package dylib

import (
	"strings"
	"testing"
)

func TestELFARMTLSAddressRecords(t *testing.T) {
	for _, descriptor := range []bool{false, true} {
		name := "traditional"
		if descriptor {
			name = "descriptor"
		}
		t.Run(name, func(t *testing.T) {
			im, o := elfTLSValidationImage()
			o.info.Arch = "arm64"
			o.relocs = []relocation{{section: 1, typ: 513, symbol: 1}, {section: 1, offset: 4, typ: 514, symbol: 1}}
			if descriptor {
				o.relocs[0].typ, o.relocs[1].typ = 562, 564
			}
			le.PutUint32(im.mem, 0x90000000)     // adrp x0, record page
			le.PutUint32(im.mem[4:], 0x91000000) // add x0, x0, record offset
			if _, err := im.layoutELFTLS(4096); err != nil {
				t.Fatal(err)
			}
			for _, r := range o.relocs {
				if err := im.relocate(o, r); err != nil {
					t.Fatal(err)
				}
			}
			page, add := le.Uint32(im.mem), le.Uint32(im.mem[4:])
			pageOffset := signExtend((page>>29&3)|(page>>5&0x7ffff)<<2, 21) << 12
			actual := uintptr(int64(im.base&^4095)+pageOffset) + uintptr(add>>10&4095)
			want := im.base + uintptr(im.elfTLS.records[definition{o, 1}])
			if descriptor {
				want += 16
			}
			if actual != want {
				t.Fatalf("address pair selects %#x, want %#x", actual, want)
			}
		})
	}
}

func TestELFARMTraditionalTLSRelocationValidation(t *testing.T) {
	for _, kind := range []string{"page", "add"} {
		for _, name := range []string{"not_enabled", "noninstruction", "addend", "opcode", "encoded_offset", "misaligned", "foreign", "non_tls", "missing_record", "bounds"} {
			t.Run(kind+"/"+name, func(t *testing.T) {
				im, o := elfTLSValidationImage()
				o.info.Arch = "arm64"
				r := relocation{section: 1, typ: 513, symbol: 1}
				ins := uint32(0x90000000)
				if kind == "add" {
					r.typ, ins = 514, 0x91000000
				}
				o.relocs = []relocation{r}
				if _, err := im.layoutELFTLS(128); err != nil {
					t.Fatal(err)
				}
				le.PutUint32(im.mem, ins)
				if err := im.relocate(o, r); err != nil {
					t.Fatal("valid traditional TLS instruction:", err)
				}
				le.PutUint32(im.mem, ins)
				switch name {
				case "not_enabled":
					im.elfTLS = nil
				case "noninstruction":
					o.sections[1].exec = false
				case "addend":
					r.addend = 1
				case "opcode":
					le.PutUint32(im.mem, 0)
				case "encoded_offset":
					le.PutUint32(im.mem, ins|1<<10)
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
				}
				if err := im.relocate(o, r); err == nil {
					t.Fatal("invalid traditional TLS relocation accepted")
				}
			})
		}
	}
}

func TestELFARMUnsupportedTLSModels(t *testing.T) {
	for _, typ := range []uint32{512, 515, 516, 517, 518, 519, 523, 539, 541, 544, 547, 549, 550, 551} {
		im, o := elfTLSValidationImage()
		o.info.Arch = "arm64"
		r := relocation{section: 1, symbol: 1, typ: typ}
		if err := im.relocate(o, r); err == nil || !strings.Contains(err.Error(), "unsupported native ELF TLS relocation") {
			t.Fatalf("unqualified relocation %d: %v", typ, err)
		}
	}
}
