package dylib

import (
	"runtime"
	"testing"
)

func elfTLSValidationImage() (*image, *object) {
	text := &section{name: "text", size: 16, exec: true, data: []byte{0x48, 0x8d, 0x3d, 0, 0, 0, 0}}
	data := &section{name: "data", size: 64, align: 8, offset: 64, write: true, nativeTLS: elfTLSData}
	o := &object{info: Info{Format: "ELF", Arch: "amd64", Bits: 64}, sections: []*section{nil, text, data}, symbols: []symbol{{}, {name: "tls", section: 2, value: 8, size: 4, tls: true, global: true}, {name: "__tls_get_addr", global: true}}}
	o.relocs = []relocation{{section: 1, offset: 3, symbol: 1, typ: 19, addend: -4, pair: -1}}
	im := &image{base: 0x10000, mem: make([]byte, 4096), pointerSize: 8, page: 4096, objects: []*object{o}, defs: map[string]definition{"tls": {o, 1}}}
	return im, o
}

func TestELFNativeTLSRuntimeAndLayoutValidation(t *testing.T) {
	for _, name := range []string{"readonly", "executable", "kind", "lifecycle", "absolute_symbol", "ordinary_symbol_section", "defined_helper", "weak_helper", "private_helper", "wrong_helper", "aliased_helper", "forwarded_helper", "empty", "size", "alignment", "budget", "descriptor_budget", "foreign_definition", "non_tls_definition", "symbol_end", "symbol_extent"} {
		t.Run(name, func(t *testing.T) {
			im, o := elfTLSValidationImage()
			switch name {
			case "readonly":
				o.sections[2].write = false
			case "executable":
				o.sections[2].exec = true
			case "kind":
				o.sections[2].nativeTLS = 3
			case "lifecycle":
				o.sections[2].lifecycle = lifecycleInit
			case "absolute_symbol":
				o.symbols[1].section = -1
			case "ordinary_symbol_section":
				o.symbols[1].section = 1
			case "defined_helper":
				o.symbols[2].section = 1
			case "weak_helper":
				o.symbols[2].weak = true
			case "private_helper":
				o.symbols[2].global = false
			case "wrong_helper":
				o.symbols[2].name = "___tls_get_addr"
			case "aliased_helper":
				o.symbols[2].alias = &weakAlias{}
			case "forwarded_helper":
				o.symbols[2].forward = &symbolForward{}
			case "empty":
				o.sections[2].size = 0
			case "size":
				o.sections[2].size = maxImage + 1
			case "alignment":
				o.sections[2].align = 3
			case "budget":
				o.sections[2].size = maxImage
			case "descriptor_budget":
				o.relocs = nil
				for n := 0; n < 4097; n++ {
					o.symbols = append(o.symbols, symbol{section: 2, tls: true, size: 1})
					o.relocs = append(o.relocs, relocation{section: 1, typ: 19, symbol: len(o.symbols) - 1})
				}
			case "foreign_definition":
				o.symbols[1].section = 0
			case "non_tls_definition":
				o.symbols[1].tls = false
			case "symbol_end":
				o.symbols[1].value = 64
			case "symbol_extent":
				o.symbols[1].size = 64
			}
			_, err := checkELFTLS(im.objects, true)
			if err == nil {
				_, err = im.layoutELFTLS(128)
			}
			if err == nil {
				t.Fatal("invalid native ELF TLS runtime/layout accepted")
			}
		})
	}
	s := New(Options{NativeTLS: true})
	defer s.Close()
	for _, name := range []string{"__tls_get_addr", "___tls_get_addr", elfTLSModuleBase} {
		err := s.Define(name, 1)
		if runtime.GOOS == "linux" && err == nil || runtime.GOOS != "linux" && err != nil {
			t.Fatal("ELF runtime reservation:", err)
		}
	}
}

func TestELFNativeTLSRelocationValidation(t *testing.T) {
	for _, arch := range []string{"amd64", "386", "arm64"} {
		for _, name := range []string{"not_enabled", "noninstruction", "addend", "opcode", "foreign", "non_tls", "missing_record"} {
			t.Run(arch+"/"+name, func(t *testing.T) {
				im, o := elfTLSValidationImage()
				o.info.Arch = arch
				o.symbols[2].name = elfTLSHelper(arch)
				r := o.relocs[0]
				switch arch {
				case "386":
					r.offset, r.typ, r.addend = 2, 18, 0
					o.sections[1].data = []byte{0x8d, 0x83, 0, 0, 0, 0}
				case "arm64":
					r.offset, r.typ, r.addend = 0, 562, 0
					o.sections[1].data = make([]byte, 4)
					le.PutUint32(o.sections[1].data, 0x90000000)
				}
				o.relocs[0] = r
				if _, err := im.layoutELFTLS(128); err != nil {
					t.Fatal(err)
				}
				copy(im.mem, o.sections[1].data)
				if err := im.relocate(o, r); err != nil {
					t.Fatal("valid entry:", err)
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
					le.PutUint32(o.sections[1].data, 0)
					copy(im.mem, o.sections[1].data)
				case "foreign":
					o.symbols[1].section = 0
				case "non_tls":
					o.symbols[1].tls = false
				case "missing_record":
					im.elfTLS.records = nil
				}
				if err := im.relocate(o, r); err == nil {
					t.Fatal("invalid ELF TLS reference accepted")
				}
			})
		}
	}
}
