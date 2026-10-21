package dylib

import "testing"

func TestELFTLSDescRelocationValidation(t *testing.T) {
	for _, arch := range []string{"amd64", "386"} {
		for _, kind := range []string{"address", "call"} {
			for _, name := range []string{"not_enabled", "noninstruction", "addend", "opcode", "foreign", "non_tls", "missing_record", "bounds"} {
				t.Run(arch+"/"+kind+"/"+name, func(t *testing.T) {
					im, o := elfTLSValidationImage()
					o.info.Arch = arch
					r := o.relocs[0]
					r.typ = 34
					if arch == "386" {
						im.pointerSize = 4
						o.info.Bits = 32
						r.typ, r.offset, r.addend = 39, 2, 0
						r.implicit = true
						o.sections[1].data = []byte{0x8d, 0x83, 0, 0, 0, 0}
					}
					if kind == "call" {
						r.typ = 35
						if arch == "386" {
							r.typ = 40
						}
						r.offset, r.addend = 0, 0
						o.sections[1].data = []byte{0xff, 0x10}
						o.sections[1].size = 2 // Annotation at the end of a section writes nothing.
					}
					o.relocs[0] = r
					if _, err := im.layoutELFTLS(128); err != nil {
						t.Fatal(err)
					}
					copy(im.mem, o.sections[1].data)
					if err := im.relocate(o, r); err != nil {
						t.Fatal("valid descriptor relocation:", err)
					}
					copy(im.mem, o.sections[1].data)
					switch name {
					case "not_enabled":
						im.elfTLS = nil
					case "noninstruction":
						o.sections[1].exec = false
					case "addend":
						r.addend++
					case "opcode":
						o.sections[1].data[0] = 0
						im.mem[0] = 0
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
						t.Fatal("invalid descriptor accepted")
					}
				})
			}
		}
	}
}

func TestELFTLSDescModuleBaseValidation(t *testing.T) {
	for _, name := range []string{"defined", "weak", "private", "non_tls", "offset", "size", "alias", "forward", "import", "arm64"} {
		t.Run(name, func(t *testing.T) {
			im, o := elfTLSValidationImage()
			o.symbols[2] = symbol{name: elfTLSModuleBase, tls: true, global: true}
			if needed, err := checkELFTLS(im.objects, true); err != nil || !needed {
				t.Fatal("valid module base:", needed, err)
			}
			s := &o.symbols[2]
			switch name {
			case "defined":
				s.section = 2
			case "weak":
				s.weak = true
			case "private":
				s.global = false
			case "non_tls":
				s.tls = false
			case "offset":
				s.value = 1
			case "size":
				s.size = 1
			case "alias":
				s.alias = &weakAlias{}
			case "forward":
				s.forward = &symbolForward{}
			case "import":
				s.imported = &coffImportSymbol{}
			case "arm64":
				o.info.Arch = "arm64"
			}
			if _, err := checkELFTLS(im.objects, true); err == nil {
				t.Fatal("invalid module base accepted")
			}
		})
	}
}
