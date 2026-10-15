package dylib

import (
	"encoding/binary"
	"fmt"
)

type windowsFunction struct {
	begin, end uint32
	data       []byte
}

// Each nested range can leave at most one resumed parent interval. Reserve
// one 16-byte empty-chain record per input in the image's 32-bit RVA window.
func (im *image) layoutWindowsUnwindProxies(size uint64) (uint64, error) {
	var count uint64
	for _, o := range im.objects {
		if o.info.Format != "COFF" || o.info.Arch != "amd64" {
			continue
		}
		for _, s := range o.sections {
			if coffPdataSection(s) {
				if s.exec || s.size%12 != 0 {
					return 0, fmt.Errorf("%s: invalid .pdata record array", o.info.Name)
				}
				count += s.size / 12
			}
		}
	}
	if count == 0 {
		return size, nil
	}
	if count > maxImage/16 || size > maxImage {
		return 0, fmt.Errorf("Windows unwind proxies exceed image limit")
	}
	start, capacity := alignUp(size, im.page), alignUp(count*16, im.page)
	if start > maxImage || capacity > maxImage-start {
		return 0, fmt.Errorf("Windows unwind proxies exceed image limit")
	}
	im.unwindProxyStart, im.unwindProxySize = start, capacity
	return start + capacity, nil
}

// All input chains have already passed x64UnwindInfo. Require the containing
// table entry itself in the child's chain, not merely a shared primary.
func (im *image) x64ChainedTo(child, parent windowsFunction) bool {
	rva := binary.LittleEndian.Uint32(child.data[8:])
	for im.mem[rva] == 0x21 {
		size := alignUp(4+2*uint64(im.mem[rva+2]), 4)
		b := im.mem[uint64(rva)+size : uint64(rva)+size+12]
		if binary.LittleEndian.Uint32(b) == parent.begin && binary.LittleEndian.Uint32(b[4:]) == parent.end && binary.LittleEndian.Uint32(b[8:]) == binary.LittleEndian.Uint32(parent.data[8:]) {
			return true
		}
		rva = binary.LittleEndian.Uint32(b[8:])
	}
	return false
}

// Split LLVM's nested ranges into disjoint intervals for RtlLookupFunctionEntry.
// A resumed parent gets an empty chain to its original entry: moving BeginAddress
// while reusing its original prolog would make the OS unwind only part of it.
func (im *image) normalizeX64Unwind(entries []windowsFunction) ([]windowsFunction, error) {
	var stack, output []windowsFunction
	var proxies []byte
	var cursor uint32
	emit := func(end uint32) error {
		if len(stack) == 0 || cursor == end {
			return nil
		}
		source := stack[len(stack)-1]
		data := append([]byte(nil), source.data...)
		binary.LittleEndian.PutUint32(data, cursor)
		binary.LittleEndian.PutUint32(data[4:], end)
		if cursor != source.begin {
			if uint64(len(proxies))+16 > im.unwindProxySize {
				return fmt.Errorf("Windows unwind proxy storage exhausted")
			}
			rva := binary.LittleEndian.Uint32(source.data[8:])
			binary.LittleEndian.PutUint32(data[8:], uint32(im.unwindProxyStart)+uint32(len(proxies)))
			proxies = append(proxies, 0x21, 0, 0, im.mem[rva+3])
			proxies = append(proxies, source.data...)
		}
		output = append(output, windowsFunction{cursor, end, data})
		cursor = end
		return nil
	}
	advance := func(limit uint32) error {
		for len(stack) != 0 && stack[len(stack)-1].end <= limit {
			if err := emit(stack[len(stack)-1].end); err != nil {
				return err
			}
			stack = stack[:len(stack)-1]
		}
		if len(stack) != 0 {
			return emit(limit)
		}
		return nil
	}
	for _, entry := range entries {
		if err := advance(entry.begin); err != nil {
			return nil, err
		}
		if len(stack) != 0 {
			parent := stack[len(stack)-1]
			rva := binary.LittleEndian.Uint32(parent.data[8:])
			if entry.begin == parent.begin || entry.end > parent.end || !im.x64ChainedTo(entry, parent) || entry.begin-parent.begin < uint32(im.mem[rva+1]) {
				return nil, fmt.Errorf("overlapping Windows runtime function entries lack a completed chained parent")
			}
		}
		stack = append(stack, entry)
		cursor = entry.begin
	}
	if err := advance(^uint32(0)); err != nil {
		return nil, err
	}
	if len(proxies) != 0 {
		if im.unwindProxyStart > uint64(len(im.mem)) || uint64(len(proxies)) > uint64(len(im.mem))-im.unwindProxyStart {
			return nil, fmt.Errorf("Windows unwind proxies are outside image storage")
		}
		copy(im.mem[im.unwindProxyStart:], proxies)
	}
	im.unwindProxyUsed = uint64(len(proxies))
	return output, nil
}
