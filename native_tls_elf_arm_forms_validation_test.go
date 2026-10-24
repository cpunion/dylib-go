package dylib

import "testing"

func TestELFARMTLSGDAddressFormValidation(t *testing.T) {
	for _, form := range []struct {
		name     string
		typ, ins uint32
	}{
		{"adr", 512, 0x10000000},
		{"movw_g1", 515, 0xd2a00000},
		{"movw_g0", 516, 0xf2800000},
	} {
		for _, name := range []string{"not_enabled", "noninstruction", "addend", "opcode", "encoded_offset", "misaligned", "foreign", "non_tls", "missing_record", "bounds", "width", "shift"} {
			t.Run(form.name+"/"+name, func(t *testing.T) {
				im, o := elfTLSValidationImage()
				o.info.Arch = "arm64"
				r := relocation{section: 1, typ: form.typ, symbol: 1}
				o.relocs = []relocation{r}
				if _, err := im.layoutELFTLS(128); err != nil {
					t.Fatal(err)
				}
				le.PutUint32(im.mem, form.ins)
				if err := im.relocate(o, r); err != nil {
					t.Fatal("valid address form:", err)
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
				case "encoded_offset":
					le.PutUint32(im.mem, form.ins|1<<5)
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
					// Clearing SF creates a 32-bit MOVW (or changes ADR to ADRP).
					le.PutUint32(im.mem, form.ins^1<<31)
				case "shift":
					le.PutUint32(im.mem, form.ins^1<<21)
				}
				if err := im.relocate(o, r); err == nil {
					t.Fatal("invalid address form accepted")
				}
			})
		}
	}
}
