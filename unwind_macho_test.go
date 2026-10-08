package dylib

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"
)

func TestMachODWARFProducerAndRebase(t *testing.T) {
	for _, triple := range []string{"x86_64-apple-macosx11", "arm64-apple-macosx11"} {
		t.Run(triple, func(t *testing.T) {
			path := compile(t, "testdata/unwind_frames.c", filepath.Join(t.TempDir(), "frames.o"), "--target="+triple, "-funwind-tables")
			b, err := readFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(path, b)
			if err != nil || len(f.info.Unsupported) != 0 {
				t.Fatalf("producer: %v, %v", f, err)
			}
			objects, err := coalesceObjects([]*object{f.obj})
			if err != nil {
				t.Fatal(err)
			}
			o := objects[0]
			defs, err := definitions(objects)
			if err != nil {
				t.Fatal(err)
			}
			im := &image{base: 0x100000, pointerSize: 8, objects: objects, defs: defs, mem: make([]byte, 4096*(len(o.sections)+1))}
			var frame *section
			var frameIndex int
			for index, s := range o.sections {
				if s == nil {
					continue
				}
				s.offset = uint64(index * 4096)
				copy(im.mem[s.offset:s.offset+s.size], s.data)
				if s.name == "__eh_frame" {
					frame, frameIndex = s, index
				}
			}
			if frame == nil || frame.size == 0 {
				t.Fatal("producer DWARF missing")
			}
			original := append([]byte(nil), frame.data...)
			explicit := 0
			for _, r := range o.relocs {
				if r.section == frameIndex {
					if err := im.relocate(o, r); err != nil {
						t.Fatal(err)
					}
					explicit++
				}
			}
			if o.info.Arch == "arm64" && explicit == 0 {
				t.Fatal("ARM64 explicit DWARF pairs missing")
			}
			if err := im.rebaseMachODWARF(); err != nil {
				t.Fatal(err)
			}
			fdes, err := im.validateDWARFFrames(im.mem[frame.offset:frame.offset+frame.size], frame.offset, true, nil)
			if err != nil || len(fdes) != 2 {
				t.Fatalf("relocated FDEs: %v, %v", fdes, err)
			}
			if !bytes.Equal(original, frame.data) {
				t.Fatal("rebase modified staged source")
			}
			if fdes[0] == im.base+uintptr(frame.offset) || fdes[0] == fdes[1] {
				t.Fatal("registration must address individual FDEs")
			}
			discardUnwind(f.obj)
			for _, s := range f.obj.sections {
				if s != nil && s.name == "__eh_frame" {
					t.Fatal("default loading retained DWARF")
				}
			}
			for _, r := range f.obj.relocs {
				if f.obj.sections[r.section] == nil {
					t.Fatal("discarded FDE dependency retained")
				}
			}
		})
	}
}

func TestMachODWARFRejectsAbsoluteEncoding(t *testing.T) {
	im := elfUnwindTestImage()
	// The existing PC-relative record is accepted by both decoders.
	fdes, err := im.validateDWARFFrames(im.mem[128:200], 128, true, nil)
	if err != nil || len(fdes) != 2 {
		t.Fatalf("PC-relative FDEs: %v", err)
	}
	im.mem[144] = 3 // absolute udata4
	for i := 0; i < 2; i++ {
		binary.LittleEndian.PutUint32(im.mem[160+i*24:], uint32(im.base)+uint32(i*32))
	}
	if _, err := im.validateDWARFFrames(im.mem[128:200], 128, false, nil); err != nil {
		t.Fatalf("ELF absolute form: %v", err)
	}
	if _, err := im.validateDWARFFrames(im.mem[128:200], 128, true, nil); err == nil {
		t.Fatal("macOS absolute FDE accepted")
	}
}
