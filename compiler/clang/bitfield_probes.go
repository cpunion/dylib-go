package clang

import (
	"bytes"
	"context"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxBitfieldProbe = 16 << 20

// C forbids offsetof on bitfields. Compile one zero-initialized constant per
// field with all its bits set, then inspect the target object in Go. This works
// for foreign targets without running compiler-produced code or parsing IR text.
func (h *Header) probeBitfields(ctx context.Context, compiler, header string, flags []string) error {
	var source strings.Builder
	var size uint64
	for _, r := range h.Records {
		if r.namedTypedefAlignment() != 0 && len(r.Bitfields) != 0 {
			source.WriteString(h.recordBodyProbes())
			break
		}
	}
	for i, r := range h.Records {
		for j, p := range r.Padding {
			if !h.probePaddingMask(r, p) {
				continue
			}
			if r.Layout.Size == 0 || r.Layout.Size > 65536 || size > maxBitfieldProbe-r.Layout.Size {
				return fmt.Errorf("clang: bitfield probes exceed storage limit")
			}
			size += r.Layout.Size
			fmt.Fprintf(&source, "%s", r.alignedPaddingProbe(i, j))
		}
		for _, f := range r.Bitfields {
			if r.Layout.Size == 0 || r.Layout.Size > 65536 || size > maxBitfieldProbe-r.Layout.Size {
				return fmt.Errorf("clang: bitfield probes exceed storage limit")
			}
			size += r.Layout.Size
			field := r.Description.Fields[f.Position]
			fmt.Fprintf(&source, "const %s dylib_bitfield_%d_%d = {.%s=%s};\n", r.Name, i, f.Position, field.Name, bitfieldValue(field.Type.Type, f.Width))
			if r.namedTypedefAlignment() != 0 {
				if size > maxBitfieldProbe-r.Layout.Size {
					return fmt.Errorf("clang: bitfield probes exceed storage limit")
				}
				size += r.Layout.Size
				fmt.Fprintf(&source, "const %s dylib_bitfield_body_%d_%d = {.%s=%s};\n", r.bodyProbeName(i), i, f.Position, field.Name, bitfieldValue(field.Type.Type, f.Width))
			}
		}
	}
	if source.Len() == 0 {
		return nil
	}
	if source.Len() > maxHeaderSize {
		return fmt.Errorf("clang: bitfield probes exceed source limit")
	}
	dir, err := os.MkdirTemp("", "dylib-bitfields-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "fields.o")
	args := append(append([]string(nil), flags...), "-x", "c", "-std=c17", "-include", header, "-fno-lto", "-c", "-o", path, "-")
	if _, err := runCompiler(ctx, compiler, args, source.String(), maxDiagnostics); err != nil {
		return err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	if stat.Size() > maxBitfieldProbe {
		return fmt.Errorf("clang: bitfield object exceeds storage limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	symbols, err := readBitfieldSymbols(data, h.Target.OS)
	if err != nil {
		return err
	}
	for i := range h.Records {
		r := &h.Records[i]
		for j := range r.Bitfields {
			f := &r.Bitfields[j]
			value, err := symbols.value(fmt.Sprintf("dylib_bitfield_%d_%d", i, f.Position), r.Layout.Size)
			if err != nil {
				return err
			}
			if r.namedTypedefAlignment() != 0 {
				bodyMask, err := symbols.value(fmt.Sprintf("dylib_bitfield_body_%d_%d", i, f.Position), r.Layout.Size)
				if err != nil || !bytes.Equal(value, bodyMask) {
					return fmt.Errorf("clang: canonical body bitfield mask mismatch for %s", r.Name)
				}
			}
			first, last := -1, -1
			for index, b := range value {
				if b != 0 {
					if first == -1 {
						first = index
					}
					last = index
				}
			}
			if first < 0 {
				return fmt.Errorf("clang: empty compiler bitfield mask")
			}
			r.Layout.Offsets[f.Position] = uint64(first)
			f.Mask = append([]byte(nil), value[first:last+1]...)
		}
		for j := range r.Padding {
			p := &r.Padding[j]
			if !h.probePaddingMask(*r, *p) {
				continue
			}
			value, err := symbols.value(fmt.Sprintf("dylib_bitfield_padding_%d_%d", i, j), r.Layout.Size)
			if err != nil {
				return err
			}
			first, last := -1, -1
			count := 0
			for index, b := range value {
				if b != 0 && first < 0 {
					first = index
				}
				if b != 0 {
					last = index
				}
				for b != 0 {
					count++
					b &= b - 1
				}
			}
			if first < 0 || count != p.Width {
				return fmt.Errorf("clang: invalid compiler unnamed bitfield mask")
			}
			p.Offset = uint64(first)
			if h.Target.gnuPacking() && h.Target.Arch == "amd64" && r.Layout.Size <= 16 {
				p.Mask = append([]byte(nil), value[first:last+1]...)
			}
			if h.Target.gnuPacking() && p.TypeAlignment != 0 {
				alignment := p.TypeAlignment
				if r.MaxFieldAlignment != 0 && alignment > r.MaxFieldAlignment {
					alignment = r.MaxFieldAlignment
				}
				if r.Packed || p.Packed {
					alignment = 1
				}
				if value[first]&1 != 0 && p.Offset%alignment == 0 {
					p.StorageAlignment = alignment
				}
				before, err := symbols.value(fmt.Sprintf("dylib_bitfield_paddingbefore_%d_%d", i, j), 8)
				if err != nil {
					return err
				}
				end := binary.LittleEndian.Uint64(before)
				if p.Offset > end {
					p.Gap = p.Offset - end
				}
			}
		}

		h.reconcileBitfieldTypedefs(r)
		if err := validRecordLayout(*r); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (h *Header) probePaddingMask(r Record, p BitfieldPadding) bool {
	return p.Width != 0 && (p.requestedAlignment() != 0 || h.Target.gnuPacking() && h.Target.Arch == "amd64" && r.Layout.Size <= 16)
}

type bitfieldSymbols map[string][]byte

func (s bitfieldSymbols) value(name string, size uint64) ([]byte, error) {
	value, found := s[name]
	if !found || size > uint64(len(value)) {
		return nil, fmt.Errorf("clang: missing or truncated compiler mask %s", name)
	}
	return value[:size], nil
}

func readBitfieldSymbols(data []byte, osName string) (bitfieldSymbols, error) {
	symbols := make(bitfieldSymbols)
	add := func(name string, size, base, offset uint64) error {
		if !strings.HasPrefix(name, "dylib_bitfield_") {
			return nil
		}
		if size > maxBitfieldProbe {
			return fmt.Errorf("clang: bitfield section exceeds storage limit")
		}
		if base > uint64(len(data)) || size > uint64(len(data))-base {
			return fmt.Errorf("clang: truncated compiler mask section")
		}
		section := data[base : base+size]
		if offset > uint64(len(section)) {
			return fmt.Errorf("clang: compiler mask outside section")
		}
		if _, duplicate := symbols[name]; duplicate {
			return fmt.Errorf("clang: duplicate compiler mask %s", name)
		}
		symbols[name] = section[offset:]
		return nil
	}
	switch osName {
	case "linux":
		f, err := elf.NewFile(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if f.Type != elf.ET_REL {
			return nil, fmt.Errorf("clang: expected ELF probe object")
		}
		syms, err := f.Symbols()
		if err != nil {
			return nil, err
		}
		for _, s := range syms {
			if int(s.Section) > 0 && int(s.Section) < len(f.Sections) {
				sec := f.Sections[s.Section]
				if err := add(s.Name, sec.Size, sec.Offset, s.Value); err != nil {
					return nil, err
				}
			}
		}
	case "darwin":
		f, err := macho.NewFile(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if f.Type != macho.TypeObj || f.Symtab == nil {
			return nil, fmt.Errorf("clang: expected Mach-O probe object")
		}
		for _, s := range f.Symtab.Syms {
			if strings.HasPrefix(s.Name, "_dylib_bitfield_") && int(s.Sect) > 0 && int(s.Sect) <= len(f.Sections) {
				sec := f.Sections[s.Sect-1]
				if s.Value < sec.Addr {
					return nil, fmt.Errorf("clang: probe symbol before section")
				}
				if err := add(strings.TrimPrefix(s.Name, "_"), sec.Size, uint64(sec.Offset), s.Value-sec.Addr); err != nil {
					return nil, err
				}
			}
		}
	case "windows":
		f, err := pe.NewFile(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if f.OptionalHeader != nil {
			return nil, fmt.Errorf("clang: expected COFF probe object")
		}
		for _, s := range f.Symbols {
			if s.SectionNumber > 0 && int(s.SectionNumber) <= len(f.Sections) {
				sec := f.Sections[s.SectionNumber-1]
				if err := add(strings.TrimPrefix(s.Name, "_"), uint64(sec.Size), uint64(sec.Offset), uint64(s.Value)); err != nil {
					return nil, err
				}
			}
		}
	default:
		return nil, fmt.Errorf("clang: unsupported bitfield probe target")
	}
	return symbols, nil
}
