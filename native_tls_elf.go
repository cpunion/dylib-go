package dylib

import (
	"fmt"

	"github.com/ramblelabs/dylib-go/internal/native"
)

const (
	elfTLSData = 1
	elfTLSZero = 2
)

// A dynamic TLS index contains a private module descriptor and byte offset.
// Each record also holds a TLSDESC [entry, index pointer] after that index.
// GD/LD and TLSDESC may therefore reference the same definition in one image.
// These records are image-owned; they are not glibc module IDs or DTV entries.
type elfTLSLayout struct {
	descriptor, template, size, align uint64
	sections                          map[*section]uint64
	records                           map[definition]uint64
	end                               uint64
	descEntry                         uintptr
	descriptors                       bool
}

const elfTLSModuleBase = "_TLS_MODULE_BASE_"

func elfTLSRuntime(name string) bool {
	return name == "__tls_get_addr" || name == "___tls_get_addr" || name == elfTLSModuleBase
}

func elfTLSDesc(arch string, typ uint32) bool {
	return arch == "amd64" && (typ == 34 || typ == 35) || arch == "386" && (typ == 39 || typ == 40) || arch == "arm64" && (typ >= 562 && typ <= 564 || typ == 569)
}

func elfTLSDescCall(arch string, typ uint32) bool {
	return arch == "amd64" && typ == 35 || arch == "386" && typ == 40
}

func elfTLSModuleBaseReference(o *object, r relocation) bool {
	return elfTLSDesc(o.info.Arch, r.typ) && r.symbol >= 0 && r.symbol < len(o.symbols) && o.symbols[r.symbol].name == elfTLSModuleBase && o.symbols[r.symbol].section == 0
}

func elfTLSHelper(arch string) string {
	if arch == "386" {
		return "___tls_get_addr" // The i386 GNU entry takes its argument in EAX.
	}
	return "__tls_get_addr"
}

func checkELFTLS(objects []*object, enabled bool) (bool, error) {
	needed := false
	for _, o := range objects {
		if o.info.Format != "ELF" {
			continue
		}
		for _, sec := range o.sections {
			if sec == nil || sec.nativeTLS == 0 {
				continue
			}
			needed = true
			if sec.nativeTLS != elfTLSData && sec.nativeTLS != elfTLSZero || !sec.write || sec.exec || sec.lifecycle != 0 {
				return false, fmt.Errorf("%s: invalid native ELF TLS section %s", o.info.Name, sec.name)
			}
		}
		for _, s := range o.symbols {
			if s.section == -3 {
				continue
			}
			if s.tls {
				needed = true
				if s.section != 0 && (s.section <= 0 || s.section >= len(o.sections) || o.sections[s.section] == nil || o.sections[s.section].nativeTLS == 0) {
					return false, fmt.Errorf("%s: TLS symbol requires an allocated TLS section", s.name)
				}
			}
			if elfTLSRuntime(s.name) {
				needed = true
				valid := s.name == elfTLSHelper(o.info.Arch) && !s.tls
				if s.name == elfTLSModuleBase {
					valid = (o.info.Arch == "amd64" || o.info.Arch == "386") && s.tls && s.value == 0 && s.size == 0
				}
				if !valid || s.section != 0 || !s.global || s.weak || s.alias != nil || s.forward != nil || s.imported != nil {
					return false, fmt.Errorf("%s: native ELF TLS requires the image-owned address helper", o.info.Name)
				}
			}
		}
	}
	if needed && !enabled {
		return false, fmt.Errorf("native ELF TLS requires Options.NativeTLS")
	}
	return needed, nil
}

// Only the dynamic models which can use image-owned storage are admitted.
// Static TP offsets and OS-loader-owned dynamic relocations are not equivalent.
func elfTLSKind(arch string, typ uint32) (record, local, offset bool) {
	switch arch {
	case "amd64":
		return typ == 19 || typ == 20 || elfTLSDesc(arch, typ), typ == 20, typ == 17 || typ == 21
	case "386":
		return typ == 18 || typ == 19 || elfTLSDesc(arch, typ), typ == 19, typ == 32
	case "arm64":
		return typ >= 562 && typ <= 564 || typ == 569, false, false
	}
	return false, false, false
}

func (im *image) elfTLSDefinition(o *object, index int) (definition, *section, error) {
	d, err := im.resolveDefinition(o, index)
	if err != nil {
		return d, nil, err
	}
	s := d.sym()
	if !s.tls || s.section <= 0 || s.section >= len(d.o.sections) || d.o.sections[s.section] == nil {
		return d, nil, fmt.Errorf("%s: native ELF TLS requires an image-owned TLS definition", s.name)
	}
	sec := d.o.sections[s.section]
	if sec.nativeTLS == 0 || s.value >= sec.size || s.size > sec.size-s.value {
		return d, nil, fmt.Errorf("%s: invalid native ELF TLS symbol extent", s.name)
	}
	return d, sec, nil
}

func (im *image) layoutELFTLS(start uint64) (uint64, error) {
	tls := &elfTLSLayout{descriptor: start, align: 1, sections: make(map[*section]uint64), records: make(map[definition]uint64)}
	for _, o := range im.objects {
		for _, sec := range o.sections {
			if sec == nil || sec.nativeTLS == 0 {
				continue
			}
			if sec.align == 0 || sec.align > im.page || sec.align&(sec.align-1) != 0 || sec.size > maxImage {
				return 0, fmt.Errorf("%s: invalid native ELF TLS alignment or size", sec.name)
			}
			tls.size = alignUp(tls.size, sec.align)
			tls.sections[sec] = tls.size
			tls.size += sec.size
			if sec.align > tls.align {
				tls.align = sec.align
			}
			if tls.size+tls.align-1 > maxImage {
				return 0, fmt.Errorf("native ELF TLS per-thread allocation budget exceeded")
			}
		}
	}
	if tls.size == 0 {
		return 0, fmt.Errorf("native ELF TLS requires an image-owned template")
	}
	tls.template = alignUp(start+4*im.pointerSize, tls.align)
	tls.end = alignUp(tls.template+tls.size, im.pointerSize)
	for _, o := range im.objects {
		for _, r := range o.relocs {
			record, local, _ := elfTLSKind(o.info.Arch, r.typ)
			if !record {
				continue
			}
			tls.descriptors = tls.descriptors || elfTLSDesc(o.info.Arch, r.typ)
			var d definition // One shared local-dynamic module-base index.
			moduleBase := elfTLSModuleBaseReference(o, r)
			if !moduleBase && (!local || r.symbol != 0) {
				var err error
				d, _, err = im.elfTLSDefinition(o, r.symbol)
				if err != nil {
					return 0, err
				}
				if local {
					d = definition{}
				}
			}
			if _, exists := tls.records[d]; !exists {
				if len(tls.records) >= 4096 {
					return 0, fmt.Errorf("native ELF TLS descriptor budget exceeded")
				}
				tls.records[d] = tls.end
				tls.end += 4 * im.pointerSize
			}
		}
	}
	tls.end = alignUp(tls.end, im.page)
	if tls.end > maxImage {
		return 0, fmt.Errorf("native ELF TLS storage exceeds image limit")
	}
	im.elfTLS = tls
	return tls.end, nil
}

func (im *image) prepareELFTLSHelper() error {
	code, err := elfTLSEntry(im.objects[0].info.Arch, native.EmulatedTLSHelper())
	if err != nil {
		return err
	}
	im.stubNext = alignUp(im.stubNext, 16)
	if len(code) > 512 || im.stubNext+uint64(len(code)) > uint64(len(im.mem)) {
		return fmt.Errorf("native ELF TLS entry exceeds stub capacity")
	}
	im.hooks[elfTLSHelper(im.objects[0].info.Arch)] = im.base + uintptr(im.stubNext)
	copy(im.mem[im.stubNext:], code)
	im.stubNext = alignUp(im.stubNext+uint64(len(code)), 16)
	im.elfTLS.descEntry = im.hooks[elfTLSHelper(im.objects[0].info.Arch)]
	if im.elfTLS.descriptors && im.objects[0].info.Arch != "arm64" {
		code, err = elfTLSDescEntry(im.objects[0].info.Arch, native.EmulatedTLSHelper())
		if err != nil {
			return err
		}
		if len(code) > 512 || im.stubNext+uint64(len(code)) > uint64(len(im.mem)) {
			return fmt.Errorf("native ELF TLSDESC entry exceeds stub capacity")
		}
		im.elfTLS.descEntry = im.base + uintptr(im.stubNext)
		copy(im.mem[im.stubNext:], code)
		im.stubNext = alignUp(im.stubNext+uint64(len(code)), 16)
	}
	return nil
}

func (im *image) prepareELFTLS() error {
	tls, word := im.elfTLS, im.pointerSize
	put := func(off, value uint64) {
		if word == 4 {
			le.PutUint32(im.mem[off:], uint32(value))
		} else {
			le.PutUint64(im.mem[off:], value)
		}
	}
	for sec, off := range tls.sections {
		copy(im.mem[tls.template+off:tls.template+off+sec.size], im.mem[sec.offset:sec.offset+sec.size])
	}
	put(tls.descriptor, tls.size)
	put(tls.descriptor+word, tls.align)
	put(tls.descriptor+3*word, uint64(im.base)+tls.template)
	if err := im.emulatedTLS.Add(im.base + uintptr(tls.descriptor)); err != nil {
		return err
	}
	for d, off := range tls.records {
		put(off+2*word, uint64(tls.descEntry))
		put(off+3*word, uint64(im.base)+off)
		put(off, uint64(im.base)+tls.descriptor)
		if d.o != nil {
			s := d.sym()
			put(off+word, tls.sections[d.o.sections[s.section]]+s.value)
		}
	}
	return nil
}

func (im *image) relocateELFTLS(o *object, r relocation, b []byte, p uintptr) (bool, error) {
	record, local, offset := elfTLSKind(o.info.Arch, r.typ)
	if !record && !offset {
		if r.symbol >= 0 && r.symbol < len(o.symbols) {
			s := o.symbols[r.symbol]
			if s.global {
				if d, ok := im.defs[s.name]; ok {
					o, s = d.o, d.sym()
				}
			}
			if s.tls || nativeTLSSection(o, s) != nil {
				return true, fmt.Errorf("unsupported native ELF TLS relocation %d", r.typ)
			}
		}
		return false, nil
	}
	if im.elfTLS == nil {
		return true, fmt.Errorf("native ELF TLS relocation requires Options.NativeTLS and an image-owned template")
	}
	tls := im.elfTLS
	var d definition
	var sec *section
	var err error
	moduleBase := elfTLSModuleBaseReference(o, r)
	if !moduleBase && (!local || r.symbol != 0) {
		d, sec, err = im.elfTLSDefinition(o, r.symbol)
		if err != nil {
			return true, err
		}
	}
	a := r.addend
	if r.implicit && !elfTLSDescCall(o.info.Arch, r.typ) {
		a += int64(int32(le.Uint32(b)))
	}
	if offset {
		v := int64(d.sym().value) + a
		if v < 0 || uint64(v) > sec.size {
			return true, fmt.Errorf("native ELF TLS offset exceeds its template section")
		}
		v += int64(tls.sections[sec])
		if len(b) == 8 {
			le.PutUint64(b, uint64(v))
			return true, nil
		}
		return true, signed32(b, v)
	}
	if local || moduleBase {
		d = definition{}
	}
	off, ok := tls.records[d]
	if !ok {
		return true, fmt.Errorf("missing native ELF TLS descriptor")
	}
	address := im.base + uintptr(off)
	if elfTLSDesc(o.info.Arch, r.typ) {
		address += uintptr(2 * im.pointerSize)
	}
	source := o.sections[r.section]
	if !source.exec {
		return true, fmt.Errorf("native ELF TLS address relocation requires an instruction section")
	}
	if elfTLSDescCall(o.info.Arch, r.typ) {
		if a != 0 || len(b) != 2 || b[0] != 0xff || b[1] != 0x10 {
			return true, fmt.Errorf("native ELF TLSDESC call requires zero-addend CALL through (E)RAX")
		}
		return true, nil // Relaxation annotation; preserve the indirect call.
	}
	switch o.info.Arch {
	case "amd64":
		if a != -4 || r.offset < 3 || r.offset > uint64(len(source.data)) {
			return true, fmt.Errorf("native ELF TLS requires a RIP-relative LEA with addend -4")
		}
		ins := source.data[r.offset-3 : r.offset]
		if ins[0] != 0x48 && ins[0] != 0x4c || ins[1] != 0x8d || ins[2]&0xc7 != 5 {
			return true, fmt.Errorf("native ELF TLS requires a 64-bit RIP-relative LEA")
		}
		return true, signed32(b, int64(address)+a-int64(p))
	case "386":
		valid := false
		if r.offset <= uint64(len(source.data)) {
			i := source.data[:r.offset]
			valid = len(i) >= 2 && i[len(i)-2] == 0x8d && i[len(i)-1]&0xc7 == 0x83 || len(i) >= 3 && i[len(i)-3] == 0x8d && i[len(i)-2]&0xc7 == 4 && i[len(i)-1] == 0x1d
		}
		if a != 0 || !valid {
			return true, fmt.Errorf("native ELF TLS requires a zero-addend EBX GOT-relative LEA")
		}
		return true, signed32(b, int64(address)-int64(im.gotBase()))
	case "arm64":
		if a != 0 || r.offset%4 != 0 {
			return true, fmt.Errorf("native ELF TLSDESC requires aligned zero-addend instructions")
		}
		ins := le.Uint32(b)
		switch r.typ {
		case 562:
			if ins&0x9f000000 != 0x90000000 || ins&0x60ffffe0 != 0 {
				return true, fmt.Errorf("TLSDESC page requires zero-addend ADRP")
			}
			return true, armPage(b, address, p)
		case 563:
			if ins&0xfffffc00 != 0xf9400000 {
				return true, fmt.Errorf("TLSDESC load requires a zero-offset 64-bit LDR")
			}
			return true, armPageOff(b, address, 3)
		case 564:
			if ins&0xfffffc00 != 0x91000000 {
				return true, fmt.Errorf("TLSDESC add requires a zero-offset 64-bit ADD")
			}
			return true, armPageOff(b, address, 0)
		case 569:
			if ins&0xfffffc1f != 0xd63f0000 || (ins>>5)&31 == 31 {
				return true, fmt.Errorf("TLSDESC call requires BLR to a general-purpose register")
			}
			return true, nil // Annotation for relaxation; instructions stay intact.
		}
	}
	return true, fmt.Errorf("unsupported native ELF TLS address relocation")
}
