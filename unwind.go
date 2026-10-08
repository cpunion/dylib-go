package dylib

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"unsafe"

	"github.com/cpunion/dylib-go/internal/native"
)

type runtimeFunctions struct {
	mem        []byte // OS-owned storage; never retain a Go heap table in Windows.
	registered bool
}

// Preserve the default linker behavior: discarded unwind metadata does not
// introduce handler dependencies or unsupported relocations. Parsed inputs
// are private to Load; Inspect keeps all sections for foreign metadata tests.
func discardCOFFUnwind(o *object) {
	if o == nil || o.info.Format != "COFF" {
		return
	}
	for i, s := range o.sections {
		if s != nil && (s.name == ".pdata" || s.name == ".xdata" || strings.HasPrefix(s.name, ".pdata$") || strings.HasPrefix(s.name, ".xdata$")) {
			o.sections[i] = nil
		}
	}
	relocs := o.relocs[:0]
	for _, r := range o.relocs {
		if o.sections[r.section] != nil {
			relocs = append(relocs, r)
		}
	}
	o.relocs = relocs
}

func (im *image) registerUnwind() error {
	arch := im.objects[0].info.Arch
	if im.objects[0].info.Format != "COFF" || arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("raw unwind registration currently requires Windows amd64/arm64")
	}
	table, err := im.windowsFunctionTable(arch)
	if err != nil || len(table) == 0 {
		return err
	}
	mem, err := native.Alloc(len(table))
	if err != nil {
		return err
	}
	copy(mem, table)
	u := &runtimeFunctions{mem: mem}
	im.unwind = u
	if err := native.Protect(mem, false, false); err != nil {
		return err
	}
	width := 12
	if arch == "arm64" {
		width = 8
	}
	if err := native.AddFunctionTable(uintptr(unsafe.Pointer(&mem[0])), uint32(len(table)/width), im.base); err != nil {
		return err
	}
	u.registered = true
	return nil
}

func (u *runtimeFunctions) close() error {
	if u == nil || u.mem == nil {
		return nil
	}
	if u.registered {
		if err := native.DeleteFunctionTable(uintptr(unsafe.Pointer(&u.mem[0]))); err != nil {
			return err
		}
		u.registered = false
	}
	if err := native.Free(u.mem); err != nil {
		return err
	}
	u.mem = nil
	return nil
}

// Decode after relocation and COMDAT selection. Input .pdata arrays can have
// different section/object orders, so publish one sorted, private native table.
func (im *image) windowsFunctionTable(arch string) ([]byte, error) {
	width := uint64(12)
	if arch == "arm64" {
		width = 8
	} else if arch != "amd64" {
		return nil, fmt.Errorf("unsupported Windows unwind architecture %s", arch)
	}
	type entry struct {
		begin, end uint32
		data       []byte
	}
	var entries []entry
	for _, o := range im.objects {
		for _, s := range o.sections {
			if s == nil || s.name != ".pdata" && !strings.HasPrefix(s.name, ".pdata$") {
				continue
			}
			if s.exec || s.size%width != 0 {
				return nil, fmt.Errorf("%s: invalid .pdata record array", o.info.Name)
			}
			for offset := uint64(0); offset < s.size; offset += width {
				b := im.mem[s.offset+offset : s.offset+offset+width]
				begin := binary.LittleEndian.Uint32(b)
				var end uint64
				if arch == "amd64" {
					end = uint64(binary.LittleEndian.Uint32(b[4:]))
				} else {
					length, err := im.arm64UnwindLength(binary.LittleEndian.Uint32(b[4:]))
					if err != nil {
						return nil, err
					}
					end = uint64(begin) + length
				}
				if end <= uint64(begin) || end > uint64(len(im.mem)) || !im.unwindRange(uint64(begin), end-uint64(begin), true) || arch == "arm64" && begin%4 != 0 {
					return nil, fmt.Errorf("%s: .pdata+%#x: function range is outside an executable section", o.info.Name, offset)
				}
				if arch == "amd64" {
					if err := im.x64UnwindInfo(binary.LittleEndian.Uint32(b[8:]), end-uint64(begin)); err != nil {
						return nil, err
					}
				}
				entries = append(entries, entry{begin, uint32(end), b})
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].begin < entries[j].begin })
	table := make([]byte, 0, uint64(len(entries))*width)
	for i, e := range entries {
		if i != 0 && e.begin < entries[i-1].end {
			return nil, fmt.Errorf("overlapping Windows runtime function entries")
		}
		table = append(table, e.data...)
	}
	return table, nil
}

func (im *image) unwindRange(offset, size uint64, exec bool) bool {
	for _, o := range im.objects {
		for _, s := range o.sections {
			if s != nil && s.exec == exec && offset >= s.offset && offset-s.offset < s.size && size <= s.size-(offset-s.offset) {
				return true
			}
		}
	}
	return false
}

func (im *image) unwindData(rva uint32, size uint64) ([]byte, error) {
	if rva%4 != 0 || !im.unwindRange(uint64(rva), size, false) {
		return nil, fmt.Errorf("unwind metadata is outside an aligned data section")
	}
	return im.mem[uint64(rva) : uint64(rva)+size], nil
}

func (im *image) x64UnwindInfo(rva uint32, length uint64) error {
	b, err := im.unwindData(rva, 4)
	if err != nil {
		return err
	}
	if b[0] != 1 { // Version 1, no language handlers or chained records.
		return fmt.Errorf("unsupported x64 unwind version/handler flags %#x", b[0])
	}
	if b[3]&15 == 0 && b[3]>>4 != 0 {
		return fmt.Errorf("invalid x64 unwind frame offset without a frame register")
	}
	count, prolog := int(b[2]), b[1]
	if uint64(prolog) > length {
		return fmt.Errorf("x64 unwind prolog exceeds its function")
	}
	b, err = im.unwindData(rva, alignUp(4+2*uint64(count), 4))
	if err != nil {
		return err
	}
	previous := prolog
	for i := 0; i < count; i++ {
		code, op := b[4+i*2], b[5+i*2]
		if code > previous {
			return fmt.Errorf("invalid x64 unwind code order/prolog offset")
		}
		previous = code
		extra := 0
		switch op & 15 {
		case 0, 2:
		case 1:
			if op>>4 > 1 {
				return fmt.Errorf("invalid x64 large-allocation encoding")
			}
			extra = 1 + int(op>>4)
		case 3:
			if op>>4 != 0 || b[3]&15 == 0 {
				return fmt.Errorf("invalid x64 frame-register encoding")
			}
		case 4, 8:
			extra = 1
		case 5, 9:
			extra = 2
		case 10:
			if op>>4 > 1 {
				return fmt.Errorf("invalid x64 machine-frame encoding")
			}
		default:
			return fmt.Errorf("unsupported x64 unwind opcode %d", op&15)
		}
		if i+extra >= count {
			return fmt.Errorf("truncated x64 unwind opcode operands")
		}
		i += extra
	}
	return nil
}

func (im *image) arm64UnwindLength(word uint32) (uint64, error) {
	flag := word & 3
	if flag == 1 || flag == 2 {
		if (word>>16)&15 > 10 {
			return 0, fmt.Errorf("invalid ARM64 packed integer-register count")
		}
		return uint64((word>>2)&0x7ff) * 4, nil
	}
	if flag == 3 {
		return 0, fmt.Errorf("reserved ARM64 packed unwind flag")
	}
	b, err := im.unwindData(word, 4)
	if err != nil {
		return 0, err
	}
	header := binary.LittleEndian.Uint32(b)
	if header&(7<<18) != 0 { // Version 0, no exception handler data.
		return 0, fmt.Errorf("unsupported ARM64 unwind version/handler flags")
	}
	epilogs, codes := uint64((header>>22)&31), uint64(header>>27)
	size := uint64(4)
	if epilogs == 0 && codes == 0 {
		b, err = im.unwindData(word, 8)
		if err != nil {
			return 0, err
		}
		ext := binary.LittleEndian.Uint32(b[4:])
		if ext>>24 != 0 {
			return 0, fmt.Errorf("reserved ARM64 extended unwind bits")
		}
		epilogs, codes, size = uint64(ext&0xffff), uint64((ext>>16)&255), 8
	}
	if codes == 0 || header&(1<<21) != 0 && epilogs >= codes*4 {
		return 0, fmt.Errorf("invalid ARM64 unwind code/epilog count")
	}
	scopes := epilogs
	if header&(1<<21) != 0 {
		scopes = 0
	}
	length := uint64(header&0x3ffff) * 4
	b, err = im.unwindData(word, size+4*(scopes+codes))
	if err != nil {
		return 0, err
	}
	var previous uint32
	for i := uint64(0); i < scopes; i++ {
		scope := binary.LittleEndian.Uint32(b[size+i*4:])
		start := scope & 0x3ffff
		if scope&(15<<18) != 0 || uint64(start)*4 >= length || uint64(scope>>22) >= codes*4 || i != 0 && start < previous {
			return 0, fmt.Errorf("invalid ARM64 unwind epilog scope")
		}
		previous = start
	}
	return length, nil
}
