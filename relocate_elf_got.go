package dylib

import (
	"fmt"
	"runtime"
)

const elfGOTBaseName = "_GLOBAL_OFFSET_TABLE_"

func elfOwnedGOT(i Info, name string) bool {
	return i.Format == "ELF" && (i.Arch == "amd64" || i.Arch == "arm64" || i.Arch == "386") && name == elfGOTBaseName
}

func hostELFOwnedGOT(name string) bool {
	return runtime.GOOS == "linux" && elfOwnedGOT(Info{Format: "ELF", Arch: runtime.GOARCH}, name)
}

func checkELFGOTDefinitions(o *object) error {
	for _, s := range o.symbols {
		if s.global && s.section != 0 && elfOwnedGOT(o.info, s.name) {
			return fmt.Errorf("%s: cannot redefine linker-owned symbol %s", o.info.Name, s.name)
		}
	}
	return nil
}

func (im *image) gotBase() uintptr { return im.base + uintptr(im.gotStart) }

// GOTPC references use the owned table base, without resolving a symbol. ELF
// parsing still validates the symbol index in each non-null relocation record.
func elfGOTBaseRelocation(o *object, typ uint32) bool {
	return o.info.Format == "ELF" && (o.info.Arch == "amd64" && (typ == 26 || typ == 29) || o.info.Arch == "386" && typ == 10)
}
