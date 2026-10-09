package dylib

import (
	"fmt"
	"runtime"
)

const elfGOTBaseName = "_GLOBAL_OFFSET_TABLE_"

func armELFOwnedGOT(i Info, name string) bool {
	return i.Format == "ELF" && i.Arch == "arm64" && name == elfGOTBaseName
}

func hostARMELFOwnedGOT(name string) bool {
	return runtime.GOOS == "linux" && armELFOwnedGOT(Info{Format: "ELF", Arch: runtime.GOARCH}, name)
}

func checkARMELFGOTDefinitions(o *object) error {
	for _, s := range o.symbols {
		if s.global && s.section != 0 && armELFOwnedGOT(o.info, s.name) {
			return fmt.Errorf("%s: cannot redefine linker-owned symbol %s", o.info.Name, s.name)
		}
	}
	return nil
}

func (im *image) gotBase() uintptr { return im.base + uintptr(im.gotStart) }

func armGOTOffset15(b []byte, slot, base, place uintptr) error {
	ins := le.Uint32(b)
	scale, err := armOffsetScale(ins)
	if err != nil || ins&0x3b000000 != 0x39000000 || scale != 3 || place&3 != 0 || ins&(1<<26) == 0 && ins&(1<<23) != 0 {
		return fmt.Errorf("GOT offset requires an aligned 64-bit unsigned-offset load/store")
	}
	offset := uint64(slot) - uint64(base)
	if offset >= 1<<15 || offset&7 != 0 {
		return fmt.Errorf("GOT offset overflow or misalignment")
	}
	le.PutUint32(b, (ins&^uint32(0xfff<<10))|uint32(offset>>3)<<10)
	return nil
}
