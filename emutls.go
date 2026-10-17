package dylib

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"github.com/cpunion/dylib-go/internal/native"
)

const emulatedTLSHelper = "__emutls_get_address"
const emulatedTLSPrefix = "__emutls_v."

func emulatedTLSReserved(name string) bool {
	return name == emulatedTLSHelper || strings.HasPrefix(name, emulatedTLSPrefix)
}

func checkEmulatedTLS(objects []*object, enabled bool) (bool, error) {
	needed := false
	for _, o := range objects {
		for _, s := range o.symbols {
			name := strings.TrimPrefix(s.name, "__imp_")
			if s.section == -3 || !emulatedTLSReserved(name) {
				continue
			}
			needed = true
			if !enabled {
				return false, fmt.Errorf("%s: emulated TLS requires Options.EmulatedTLS", o.info.Name)
			}
			if s.imported != nil || name == emulatedTLSHelper && s.section != 0 {
				return false, fmt.Errorf("%s: emulated TLS requires image-owned descriptors and the host helper", o.info.Name)
			}
		}
	}
	if needed {
		for _, o := range objects {
			for _, s := range o.symbols {
				switch strings.TrimPrefix(s.name, "__imp_") {
				case "__cxa_thread_atexit", "__cxa_thread_atexit_impl", "_tlv_atexit", "__tlv_atexit", "_tlregdtor", "__tlregdtor", "__dyn_tls_on_demand_init":
					return false, fmt.Errorf("%s: emulated TLS thread destructors/dynamic runtime initialization are unsupported", o.info.Name)
				}
			}
		}
	}
	return needed, nil
}

// Validate the relocated compiler descriptors before any native initialization.
// No native allocator or runtime helper reads an unvalidated descriptor.
func (im *image) emulatedTLSDescriptors() ([]uintptr, error) {
	word := im.pointerSize
	if word != 4 && word != 8 {
		return nil, fmt.Errorf("invalid emulated TLS pointer size")
	}
	contains := func(address, size uint64, writable bool) bool {
		if address < uint64(im.base) {
			return false
		}
		offset := address - uint64(im.base)
		for _, o := range im.objects {
			for _, sec := range o.sections {
				if sec != nil && !sec.exec && (!writable || sec.write) && offset >= sec.offset && offset-sec.offset <= sec.size && size <= sec.size-(offset-sec.offset) {
					return offset <= uint64(len(im.mem)) && size <= uint64(len(im.mem))-offset
				}
			}
		}
		return false
	}
	read := func(offset uint64) uint64 {
		if word == 4 {
			return uint64(binary.LittleEndian.Uint32(im.mem[offset:]))
		}
		return binary.LittleEndian.Uint64(im.mem[offset:])
	}
	seen := make(map[uintptr]bool)
	var descriptors []uintptr
	var budget uint64
	for _, o := range im.objects {
		for i, s := range o.symbols {
			if s.section == -3 || !strings.HasPrefix(s.name, emulatedTLSPrefix) {
				continue
			}
			d, err := im.resolveDefinition(o, i)
			if err != nil {
				return nil, err
			}
			v := d.sym()
			if v.section <= 0 {
				return nil, fmt.Errorf("%s: emulated TLS descriptor must belong to the image", s.name)
			}
			address, err := im.symbol(d.o, d.index, false)
			if err != nil {
				return nil, err
			}
			if seen[address] {
				continue
			}
			if uint64(address)%word != 0 || !contains(uint64(address), 4*word, true) || v.size != 0 && v.size != 4*word {
				return nil, fmt.Errorf("%s: invalid emulated TLS descriptor storage", s.name)
			}
			offset := uint64(address - im.base)
			size, alignment, object, initial := read(offset), read(offset+word), read(offset+2*word), read(offset+3*word)
			if size == 0 || size > maxImage || alignment == 0 || alignment > maxImage || alignment&(alignment-1) != 0 || object != 0 {
				return nil, fmt.Errorf("%s: invalid emulated TLS descriptor fields", s.name)
			}
			if initial != 0 && !contains(initial, size, false) {
				return nil, fmt.Errorf("%s: emulated TLS template must belong to image data", s.name)
			}
			budget += size + alignment - 1
			if budget > maxImage || len(descriptors) >= 4096 {
				return nil, fmt.Errorf("emulated TLS per-thread allocation budget exceeded")
			}
			seen[address] = true
			descriptors = append(descriptors, address)
		}
	}
	sort.Slice(descriptors, func(i, j int) bool { return descriptors[i] < descriptors[j] })
	for i := 1; i < len(descriptors); i++ {
		if descriptors[i]-descriptors[i-1] < uintptr(4*word) {
			return nil, fmt.Errorf("overlapping emulated TLS descriptors")
		}
	}
	if len(descriptors) == 0 {
		return nil, fmt.Errorf("emulated TLS helper requires an image-owned descriptor")
	}
	return descriptors, nil
}

func (im *image) prepareEmulatedTLS() error {
	descriptors, err := im.emulatedTLSDescriptors()
	if err != nil {
		return err
	}
	im.emulatedTLS, err = native.NewEmulatedTLS()
	if err != nil {
		return err
	}
	for _, address := range descriptors {
		if err := im.emulatedTLS.Add(address); err != nil {
			return err
		}
	}
	return nil
}
