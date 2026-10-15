package dylib

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func overlapUnwindTestImage(t *testing.T) *image {
	t.Helper()
	im := chainedUnwindTestImage()
	le := binary.LittleEndian
	le.PutUint32(im.mem[128:], 8)
	le.PutUint32(im.mem[132:], 16)
	le.PutUint32(im.mem[136:], 72)
	im.page = 16
	size, err := im.layoutWindowsUnwindProxies(uint64(len(im.mem)))
	if err != nil {
		t.Fatal(err)
	}
	mem := make([]byte, size)
	copy(mem, im.mem)
	im.mem = mem
	return im
}

func TestWindowsOverlappingUnwindIntervals(t *testing.T) {
	im := overlapUnwindTestImage(t)
	original := append([]byte(nil), im.mem[:256]...)
	table, err := im.windowsFunctionTable("amd64")
	if err != nil || len(table) != 36 || im.unwindProxyUsed != 16 {
		t.Fatalf("normalized table: %x, proxies=%d, %v", table, im.unwindProxyUsed, err)
	}
	for i, want := range [][3]uint32{{0, 8, 64}, {8, 16, 72}, {16, 32, uint32(im.unwindProxyStart)}} {
		b := table[i*12:]
		begin, end, rva := le.Uint32(b), le.Uint32(b[4:]), le.Uint32(b[8:])
		if [3]uint32{begin, end, rva} != want {
			t.Fatalf("interval %d: %x; want %v", i, b[:12], want)
		}
		if err := im.x64UnwindInfo(rva, uint64(end-begin)); err != nil {
			t.Fatal(err)
		}
	}
	proxy := im.mem[im.unwindProxyStart : im.unwindProxyStart+16]
	if !bytes.Equal(proxy[:4], []byte{0x21, 0, 0, 0}) || !bytes.Equal(proxy[4:], original[140:152]) {
		t.Fatalf("resumed parent did not chain to the original entry: %x", proxy)
	}
	if !bytes.Equal(original, im.mem[:256]) {
		t.Fatal("normalization modified input sections")
	}
	again, err := im.windowsFunctionTable("amd64")
	if err != nil || !bytes.Equal(table, again) {
		t.Fatalf("non-deterministic table rebuild: %x, %v", again, err)
	}
}

func TestWindowsOverlappingUnwindFramesAndSiblings(t *testing.T) {
	t.Run("frame register", func(t *testing.T) {
		im := overlapUnwindTestImage(t)
		copy(im.mem[64:], []byte{1, 4, 2, 0x25, 4, 3, 4, 0x32})
		im.mem[75] = 0x25
		if _, err := im.windowsFunctionTable("amd64"); err != nil {
			t.Fatal(err)
		}
		if im.mem[im.unwindProxyStart+3] != 0x25 {
			t.Fatal("proxy lost frame register/offset")
		}
		if err := im.x64UnwindInfo(uint32(im.unwindProxyStart), 16); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("same end", func(t *testing.T) {
		im := overlapUnwindTestImage(t)
		le.PutUint32(im.mem[132:], 32)
		if table, err := im.windowsFunctionTable("amd64"); err != nil || len(table) != 24 || im.unwindProxyUsed != 0 {
			t.Fatal(table, im.unwindProxyUsed, err)
		}
	})
	t.Run("siblings", func(t *testing.T) {
		im := chainedUnwindTestImage()
		im.objects[0].sections[3].size = 36
		for i, row := range [][3]uint32{{24, 32, 92}, {0, 64, 64}, {8, 16, 72}} {
			for j, v := range row {
				le.PutUint32(im.mem[128+i*12+j*4:], v)
			}
		}
		le.PutUint32(im.mem[84:], 64)
		le.PutUint32(im.mem[100:], 0)
		le.PutUint32(im.mem[104:], 64)
		le.PutUint32(im.mem[108:], 64)
		im.page = 16
		size, err := im.layoutWindowsUnwindProxies(256)
		if err != nil {
			t.Fatal(err)
		}
		im.mem = append(im.mem, make([]byte, int(size)-len(im.mem))...)
		table, err := im.windowsFunctionTable("amd64")
		if err != nil || len(table) != 60 || im.unwindProxyUsed != 32 {
			t.Fatal(table, im.unwindProxyUsed, err)
		}
		for i, want := range [][2]uint32{{0, 8}, {8, 16}, {16, 24}, {24, 32}, {32, 64}} {
			b := table[i*12:]
			if [2]uint32{le.Uint32(b), le.Uint32(b[4:])} != want {
				t.Fatal("incorrect sibling intervals", table)
			}
			if err := im.x64UnwindInfo(le.Uint32(b[8:]), uint64(want[1]-want[0])); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestWindowsOverlappingUnwindValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch func(*image)
	}{
		{"crossing ranges", func(im *image) { le.PutUint32(im.mem[132:], 40) }},
		{"same start", func(im *image) { le.PutUint32(im.mem[128:], 0) }},
		{"unrelated records", func(im *image) { im.mem[72] = 1 }},
		{"different parent start", func(im *image) { le.PutUint32(im.mem[80:], 4) }},
		{"different parent end", func(im *image) { le.PutUint32(im.mem[84:], 24) }},
		{"incomplete parent prolog", func(im *image) { le.PutUint32(im.mem[128:], 2) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			im := overlapUnwindTestImage(t)
			tc.patch(im)
			if _, err := im.windowsFunctionTable("amd64"); err == nil || !strings.Contains(err.Error(), "overlapping") {
				t.Fatalf("invalid overlap: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		name  string
		patch func(*image)
	}{
		{"absent storage", func(im *image) { im.unwindProxySize = 0 }},
		{"short storage", func(im *image) { im.unwindProxySize = 8 }},
		{"outside image", func(im *image) { im.unwindProxyStart = uint64(len(im.mem)) + 16 }},
		{"outside RVA window", func(im *image) { im.unwindProxyStart = 1 << 32 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			im := overlapUnwindTestImage(t)
			tc.patch(im)
			if _, err := im.windowsFunctionTable("amd64"); err == nil {
				t.Fatal("missing proxy storage accepted")
			}
		})
	}
}

func TestWindowsUnwindProxyLayout(t *testing.T) {
	im := unwindTestImage("amd64")
	im.page = 4096
	size, err := im.layoutWindowsUnwindProxies(256)
	if err != nil || size != 8192 || im.unwindProxyStart != 4096 || im.unwindProxySize != 4096 {
		t.Fatal(size, im.unwindProxyStart, im.unwindProxySize, err)
	}
	for _, n := range []uint64{maxImage - 1, maxImage + 1} {
		im := unwindTestImage("amd64")
		im.page = 4096
		if _, err := im.layoutWindowsUnwindProxies(n); err == nil {
			t.Fatal("oversized proxy image accepted")
		}
	}
	im = unwindTestImage("amd64")
	im.page = 4096
	im.objects[0].sections[3].size = 12 * (maxImage/16 + 1)
	if _, err := im.layoutWindowsUnwindProxies(0); err == nil {
		t.Fatal("oversized proxy count accepted")
	}
	im = unwindTestImage("amd64")
	im.page = 4096
	im.objects[0].sections[3].size--
	if _, err := im.layoutWindowsUnwindProxies(0); err == nil {
		t.Fatal("malformed pdata reservation accepted")
	}
	for _, arch := range []string{"arm64", "386"} {
		im := unwindTestImage("arm64")
		im.objects[0].info.Arch = arch
		im.page = 4096
		if size, err := im.layoutWindowsUnwindProxies(256); err != nil || size != 256 || im.unwindProxySize != 0 {
			t.Fatal(size, err)
		}
	}
}
