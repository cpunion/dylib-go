package dylib

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func machoTLSValidationImage() (*image, *object) {
	text := &section{name: "text", size: 16, exec: true, data: []byte{0x48, 0x8b, 0x3d, 0, 0, 0, 0}}
	data := &section{name: "data", size: 64, align: 8, offset: 64, write: true, nativeTLS: 0x11}
	vars := &section{name: "vars", size: 24, align: 1, offset: 128, write: true, nativeTLS: 0x13}
	o := &object{info: Info{Format: "Mach-O", OS: "darwin", Arch: "amd64", Bits: 64}, sections: []*section{nil, text, data, vars}, symbols: []symbol{{name: machoTLSBootstrap, global: true}, {name: "initial", section: 2}, {name: "tls", section: 3, global: true}}}
	o.relocs = []relocation{{section: 3, offset: 0, width: 8, pair: -1}, {section: 3, offset: 16, width: 8, symbol: 1, pair: -1}}
	im := &image{base: 0x10000, mem: make([]byte, 512), pointerSize: 8, page: 4096, objects: []*object{o}, defs: map[string]definition{"tls": {o, 2}}, hooks: map[string]uintptr{machoTLSBootstrap: 0x20000}, machoTLS: &machoTLSLayout{descriptor: 192, template: 224, size: 64, align: 8, sections: map[*section]uint64{data: 0}}}
	le.PutUint64(im.mem[128:], 0x20000)
	le.PutUint64(im.mem[144:], uint64(im.base)+64)
	return im, o
}

func TestMachONativeTLSDescriptorValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*image, *object)
	}{
		{"missing_helper", func(im *image, _ *object) { im.hooks = nil }},
		{"wrong_helper", func(im *image, _ *object) { le.PutUint64(im.mem[128:], 1) }},
		{"key_in_use", func(im *image, _ *object) { le.PutUint64(im.mem[136:], 1) }},
		{"foreign_template", func(im *image, _ *object) { le.PutUint64(im.mem[144:], uint64(im.base)-1) }},
		{"template_end", func(im *image, _ *object) { le.PutUint64(im.mem[144:], uint64(im.base)+128) }},
		{"ordinary_data", func(im *image, _ *object) { le.PutUint64(im.mem[144:], uint64(im.base)+16) }},
		{"no_bootstrap_relocation", func(_ *image, o *object) { o.relocs = o.relocs[1:] }},
		{"no_initial_relocation", func(_ *image, o *object) { o.relocs = o.relocs[:1] }},
		{"wrong_bootstrap_symbol", func(_ *image, o *object) { o.relocs[0].symbol = 1 }},
		{"duplicate", func(_ *image, o *object) { o.relocs = append(o.relocs, o.relocs[0]) }},
		{"key_relocation", func(_ *image, o *object) { o.relocs[0].offset = 8 }},
		{"width", func(_ *image, o *object) { o.relocs[0].width = 4 }},
		{"relative", func(_ *image, o *object) { o.relocs[0].pcrel = true }},
		{"subtractor", func(_ *image, o *object) { o.relocs[0].pair = 1 }},
		{"out_of_range_relocation", func(_ *image, o *object) { o.relocs[0].offset = 24 }},
		{"misaligned", func(_ *image, o *object) { o.sections[3].offset++ }},
		{"truncated", func(_ *image, o *object) { o.sections[3].size-- }},
		{"outside_image", func(_ *image, o *object) { o.sections[3].offset = 512 }},
		{"pointer_size", func(im *image, _ *object) { im.pointerSize = 4 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			im, o := machoTLSValidationImage()
			if patches, err := im.machOTLSDescriptors(); err != nil || len(patches) != 1 || patches[0].offset != 0 {
				t.Fatal(patches, err)
			}
			tc.change(im, o)
			if _, err := im.machOTLSDescriptors(); err == nil {
				t.Fatal("malformed native TLV descriptor accepted")
			}
		})
	}
}

func TestMachONativeTLSReservedAndLayout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*image, *object)
	}{
		{"readonly", func(_ *image, o *object) { o.sections[3].write = false }},
		{"executable", func(_ *image, o *object) { o.sections[2].exec = true }},
		{"initializer_kind", func(_ *image, o *object) { o.sections[2].nativeTLS = 0x15 }},
		{"pointer_kind", func(_ *image, o *object) { o.sections[2].nativeTLS = 0x14 }},
		{"foreign_os", func(_ *image, o *object) { o.info.OS = "linux" }},
		{"foreign_format", func(_ *image, o *object) { o.info.Format = "ELF" }},
		{"defined_bootstrap", func(_ *image, o *object) { o.symbols[0].section = 1 }},
		{"weak_bootstrap", func(_ *image, o *object) { o.symbols[0].weak = true }},
		{"private_bootstrap", func(_ *image, o *object) { o.symbols[0].global = false }},
		{"imported_bootstrap", func(_ *image, o *object) { o.symbols[0].imported = &coffImportSymbol{} }},
		{"alias_bootstrap", func(_ *image, o *object) { o.symbols[0].alias = &weakAlias{} }},
		{"truncated", func(_ *image, o *object) { o.sections[3].size-- }},
		{"descriptor_budget", func(_ *image, o *object) { o.sections[3].size = 4097 * 24 }},
		{"byte_budget", func(_ *image, o *object) { o.sections[2].size = maxImage }},
		{"alignment", func(_ *image, o *object) { o.sections[2].align = 3 }},
		{"no_template", func(_ *image, o *object) { o.sections[2].size = 0 }},
		{"no_descriptors", func(_ *image, o *object) { o.sections[3].size = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			im, o := machoTLSValidationImage()
			tc.change(im, o)
			_, err := checkMachOTLS(im.objects, true)
			if err == nil {
				_, err = im.layoutMachOTLS(0)
			}
			if err == nil {
				t.Fatal("invalid native TLS runtime/layout accepted")
			}
		})
	}
	s := New(Options{NativeTLS: true})
	defer s.Close()
	for _, name := range []string{machoTLSBootstrap, "__imp_" + machoTLSBootstrap} {
		err := s.Define(name, 1)
		if runtime.GOOS == "darwin" && err == nil || runtime.GOOS != "darwin" && err != nil {
			t.Fatal("platform bootstrap reservation:", err)
		}
	}
}

func TestMachONativeTLSRelocationValidation(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		for _, name := range []string{"target_kind", "target_offset", "outside_descriptor", "foreign_target", "local", "addend", "width", "noninstruction", "embedded_addend", "opcode"} {
			t.Run(arch+"/"+name, func(t *testing.T) {
				im, o := machoTLSValidationImage()
				o.info.Arch = arch
				r := relocation{section: 1, offset: 3, symbol: 2, typ: 9, pcrel: true, width: 4, pair: -1}
				b := make([]byte, 4)
				if arch == "arm64" {
					r.offset, r.typ = 0, 8
					le.PutUint32(b, 0x90000000)
				}
				if _, err := im.machOTLSRelocation(o, r, b); err != nil {
					t.Fatal(err)
				}
				switch name {
				case "target_kind":
					o.symbols[2].section = 2
				case "target_offset":
					o.symbols[2].value = 1
				case "outside_descriptor":
					o.symbols[2].value = 24
				case "foreign_target":
					o.symbols[2].section = 0
				case "local":
					r.local = true
				case "addend":
					r.addend = 1
				case "width":
					r.width = 8
				case "noninstruction":
					o.sections[1].exec = false
				case "embedded_addend":
					if arch == "amd64" {
						le.PutUint32(b, 1)
					} else {
						le.PutUint32(b, 0xb0000000)
					}
				case "opcode":
					if arch == "amd64" {
						o.sections[1].data[1] = 0x8d
					} else {
						le.PutUint32(b, 0x14000000)
					}
				}
				if _, err := im.machOTLSRelocation(o, r, b); err == nil {
					t.Fatal("unsafe TLVP reference accepted")
				}
			})
		}
	}
}

func TestMachONativeTLSUnsupportedSections(t *testing.T) {
	for _, target := range []string{"x86_64-apple-macosx11", "arm64-apple-macosx11"} {
		t.Run(target, func(t *testing.T) {
			path := compile(t, "testdata/emutls_consumer.c", filepath.Join(t.TempDir(), "consumer.o"), "--target="+target, "-ffreestanding")
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Header flags, not the section spelling, define pointer/init models.
			for _, kind := range []uint32{0x14, 0x15} {
				copy := append([]byte(nil), b...)
				header := machoPointerHeader(t, copy, "__thread_vars")
				le.PutUint32(copy[header+64:], kind)
				parsed, err := parse("unsupported-tls", copy)
				if err != nil || len(parsed.info.Unsupported) == 0 {
					t.Fatal("unsupported native TLS model accepted:", err)
				}
			}
		})
	}
}
