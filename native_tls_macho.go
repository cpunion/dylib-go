package dylib

import (
	"fmt"

	"github.com/ramblelabs/dylib-go/internal/native"
)

const machoTLSBootstrap = "_tlv_bootstrap" // One Mach-O linker underscore is removed.

type machoTLSLayout struct {
	descriptor, template, size, align uint64
	sections                          map[*section]uint64
}

type machoTLVPatch struct{ descriptor, offset uint64 }

func machoTLVDescriptorRelocation(r relocation) bool {
	return r.typ == 0 && r.width == 8 && !r.pcrel && r.pair < 0 && (r.offset%24 == 0 || r.offset%24 == 16)
}

func checkMachOTLS(objects []*object, enabled bool) (bool, error) {
	needed := false
	for _, o := range objects {
		if o.info.Format != "Mach-O" {
			continue
		}
		for _, sec := range o.sections {
			if sec == nil || sec.nativeTLS == 0 {
				continue
			}
			needed = true
			if o.info.Format != "Mach-O" || o.info.OS != "darwin" || o.info.Bits != 64 || o.info.Arch != "amd64" && o.info.Arch != "arm64" || sec.nativeTLS < 0x11 || sec.nativeTLS > 0x13 || sec.exec || !sec.write || sec.lifecycle != 0 {
				return false, fmt.Errorf("%s: invalid native Mach-O TLS section %s", o.info.Name, sec.name)
			}
		}
		for _, s := range o.symbols {
			if o.info.Format != "Mach-O" || s.section == -3 || s.name != machoTLSBootstrap {
				continue
			}
			needed = true
			if o.info.OS != "darwin" || s.section != 0 || !s.global || s.weak || s.alias != nil || s.forward != nil || s.imported != nil {
				return false, fmt.Errorf("%s: native TLS requires the image-owned bootstrap", o.info.Name)
			}
		}
	}
	if needed && !enabled {
		return false, fmt.Errorf("native Mach-O TLS requires Options.NativeTLS")
	}
	return needed, nil
}

func (im *image) layoutMachOTLS(start uint64) (uint64, error) {
	tls := &machoTLSLayout{align: 1, descriptor: start, sections: make(map[*section]uint64)}
	var descriptors uint64
	for _, o := range im.objects {
		for _, sec := range o.sections {
			if sec == nil || sec.nativeTLS == 0 {
				continue
			}
			if sec.nativeTLS == 0x13 {
				if sec.size%24 != 0 {
					return 0, fmt.Errorf("%s: truncated native TLV descriptor", sec.name)
				}
				descriptors += sec.size / 24
				if descriptors > 4096 {
					return 0, fmt.Errorf("native TLV descriptor budget exceeded")
				}
				continue
			}
			if sec.align == 0 || sec.align > im.page || sec.align&(sec.align-1) != 0 || sec.size > maxImage {
				return 0, fmt.Errorf("%s: invalid native TLS template layout", sec.name)
			}
			tls.size = alignUp(tls.size, sec.align)
			tls.sections[sec] = tls.size
			tls.size += sec.size
			if sec.align > tls.align {
				tls.align = sec.align
			}
			if tls.size+tls.align-1 > maxImage {
				return 0, fmt.Errorf("native TLS per-thread allocation budget exceeded")
			}
		}
	}
	if descriptors == 0 || tls.size == 0 {
		return 0, fmt.Errorf("native TLS requires image-owned TLV descriptors and templates")
	}
	// Private native storage, never exported under an artificial symbol name.
	tls.template = alignUp(start+32, tls.align)
	end := alignUp(tls.template+tls.size, im.page)
	if end > maxImage {
		return 0, fmt.Errorf("native TLS storage exceeds image limit")
	}
	im.machoTLS = tls
	return end, nil
}

// Normalize only proven image-owned TLV references. A template address is not
// a per-thread variable address, and a foreign pointer is not a TLV descriptor.
func (im *image) machOTLSRelocation(o *object, r relocation, b []byte) (relocation, error) {
	tlv := o.info.Arch == "amd64" && r.typ == 9 || o.info.Arch == "arm64" && (r.typ == 8 || r.typ == 9)
	if im.machoTLS == nil && !tlv {
		return r, nil
	}
	source := o.sections[r.section]
	if source.nativeTLS == 0x13 && !machoTLVDescriptorRelocation(r) {
		return r, fmt.Errorf("invalid native TLV descriptor relocation")
	}
	var target *section
	var value uint64
	if r.local {
		if r.symbol <= 0 || r.symbol >= len(o.sections) || o.sections[r.symbol] == nil {
			return r, fmt.Errorf("invalid native TLS section reference")
		}
		target = o.sections[r.symbol]
	} else {
		d, err := im.resolveDefinition(o, r.symbol)
		if err != nil {
			return r, err
		}
		target, value = nativeTLSSection(d.o, d.sym()), d.sym().value
	}
	if tlv {
		if im.machoTLS == nil || r.local || r.width != 4 || r.addend != 0 || r.pair >= 0 || target == nil || target.nativeTLS != 0x13 || value%24 != 0 || value >= target.size || target.size-value < 24 {
			return r, fmt.Errorf("TLVP reference requires an image-owned native TLV descriptor")
		}
		if !source.exec {
			return r, fmt.Errorf("TLVP reference requires an instruction section")
		}
		if o.info.Arch == "amd64" {
			if !r.pcrel || le.Uint32(b) != 0 || r.offset < 3 || r.offset > uint64(len(source.data)) {
				return r, fmt.Errorf("native TLVP requires a zero-addend RIP-relative MOV load")
			}
			instruction := source.data[r.offset-3 : r.offset]
			if instruction[0] != 0x48 && instruction[0] != 0x4c || instruction[1] != 0x8b || instruction[2]&0xc7 != 5 {
				return r, fmt.Errorf("native TLVP requires a 64-bit RIP-relative MOV load")
			}
			r.typ = 4 // X86_64_RELOC_TLV -> GOT.
		} else {
			instruction := le.Uint32(b)
			if r.typ == 8 && (!r.pcrel || instruction&0x9f000000 != 0x90000000 || instruction&0x60ffffe0 != 0) || r.typ == 9 && (r.pcrel || instruction&0xfffffc00 != 0xf9400000) {
				return r, fmt.Errorf("native TLVP requires zero-addend ADRP/LDR address loads")
			}
			r.typ -= 3 // TLVP_LOAD_PAGE21/PAGEOFF12 -> GOT_LOAD_PAGE21/PAGEOFF12.
		}
		return r, nil
	}
	if target != nil && target.nativeTLS != 0 {
		if target.nativeTLS == 0x13 || source.nativeTLS != 0x13 || r.offset%24 != 16 {
			return r, fmt.Errorf("native TLS template addresses require a TLV descriptor; variable references require TLVP")
		}
	}
	return r, nil
}

func (im *image) machOTLSDescriptors() ([]machoTLVPatch, error) {
	tls := im.machoTLS
	if tls == nil || im.pointerSize != 8 || im.hooks[machoTLSBootstrap] == 0 {
		return nil, fmt.Errorf("native TLV descriptors require a native image-owned bootstrap")
	}
	var patches []machoTLVPatch
	for _, o := range im.objects {
		for index, sec := range o.sections {
			if sec == nil || sec.nativeTLS != 0x13 {
				continue
			}
			if sec.size%24 != 0 || sec.offset > uint64(len(im.mem)) || sec.size > uint64(len(im.mem))-sec.offset {
				return nil, fmt.Errorf("%s: invalid native TLV descriptor storage", sec.name)
			}
			slots := make(map[uint64]bool)
			for _, r := range o.relocs {
				if r.section != index {
					continue
				}
				if slots[r.offset] || !machoTLVDescriptorRelocation(r) || r.offset >= sec.size || uint64(r.width) > sec.size-r.offset {
					return nil, fmt.Errorf("%s: invalid or overlapping native TLV descriptor relocations", sec.name)
				}
				slots[r.offset] = true
				if r.offset%24 == 0 && (r.local || r.symbol < 0 || r.symbol >= len(o.symbols) || o.symbols[r.symbol].name != machoTLSBootstrap) {
					return nil, fmt.Errorf("%s: native TLV descriptor requires the image-owned bootstrap relocation", sec.name)
				}
			}
			for off := uint64(0); off < sec.size; off += 24 {
				address := sec.offset + off
				if (uint64(im.base)+address)%8 != 0 || !slots[off] || !slots[off+16] || le.Uint64(im.mem[address:]) != uint64(im.hooks[machoTLSBootstrap]) || le.Uint64(im.mem[address+8:]) != 0 {
					return nil, fmt.Errorf("%s: invalid native TLV descriptor fields or storage", sec.name)
				}
				initial := le.Uint64(im.mem[address+16:])
				found := false
				for template, offset := range tls.sections {
					base := uint64(im.base) + template.offset
					if initial >= base && initial-base < template.size {
						patches = append(patches, machoTLVPatch{address, offset + initial - base})
						found = true
						break
					}
				}
				if !found {
					return nil, fmt.Errorf("%s: native TLV template must belong to image TLS data", sec.name)
				}
			}
		}
	}
	return patches, nil
}

func (im *image) prepareMachOTLS() error {
	patches, err := im.machOTLSDescriptors()
	if err != nil {
		return err
	}
	tls := im.machoTLS
	for sec, offset := range tls.sections {
		copy(im.mem[tls.template+offset:tls.template+offset+sec.size], im.mem[sec.offset:sec.offset+sec.size])
	}
	le.PutUint64(im.mem[tls.descriptor:], tls.size)
	le.PutUint64(im.mem[tls.descriptor+8:], tls.align)
	le.PutUint64(im.mem[tls.descriptor+24:], uint64(im.base)+tls.template)
	if err := im.emulatedTLS.Add(im.base + uintptr(tls.descriptor)); err != nil {
		return err
	}
	for _, p := range patches {
		le.PutUint64(im.mem[p.descriptor+8:], uint64(im.base)+tls.descriptor)
		le.PutUint64(im.mem[p.descriptor+16:], p.offset)
	}
	return nil
}

func (im *image) prepareMachOTLSHelper() error {
	code, err := machoTLSEntry(im.objects[0].info.Arch, native.EmulatedTLSHelper())
	if err != nil {
		return err
	}
	im.stubNext = alignUp(im.stubNext, 16)
	if len(code) > 512 || im.stubNext+uint64(len(code)) > uint64(len(im.mem)) {
		return fmt.Errorf("native TLV entry exceeds stub capacity")
	}
	im.hooks[machoTLSBootstrap] = im.base + uintptr(im.stubNext)
	copy(im.mem[im.stubNext:], code)
	im.stubNext = alignUp(im.stubNext+uint64(len(code)), 16)
	return nil
}
