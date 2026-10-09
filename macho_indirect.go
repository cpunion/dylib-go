package dylib

import (
	"bytes"
	"debug/macho"
	"fmt"
	"strings"
)

type machoIndirect struct {
	index  int
	target string
}

func machoIndirectTarget(f *macho.File, data []byte, s macho.Symbol) (string, error) {
	// debug/macho retains LC_SYMTAB bytes but does not populate SymtabCmd.
	cmd := f.Symtab.Raw()
	if len(cmd) < 24 {
		return "", fmt.Errorf("Mach-O indirect symbol %s: truncated symbol table command", s.Name)
	}
	start, size := uint64(f.ByteOrder.Uint32(cmd[16:])), uint64(f.ByteOrder.Uint32(cmd[20:]))
	if s.Sect != 0 || start > uint64(len(data)) || size > uint64(len(data))-start || s.Value >= size {
		return "", fmt.Errorf("Mach-O indirect symbol %s: invalid target string offset or section", s.Name)
	}
	b := data[start+s.Value : start+size]
	end := bytes.IndexByte(b, 0)
	if end < 0 {
		return "", fmt.Errorf("Mach-O indirect symbol %s: unterminated target string", s.Name)
	}
	name := strings.TrimPrefix(string(b[:end]), "_")
	if name == "" {
		return "", fmt.Errorf("Mach-O indirect symbol %s: empty target name", s.Name)
	}
	return name, nil
}

func bindMachOIndirect(o *object, indirect []machoIndirect) {
	if len(indirect) == 0 {
		return
	}
	// N_INDR targets use external names. Local symbols with the same spelling
	// cannot satisfy them. Keep original nlist indices for all relocations;
	// append a synthetic undefined external only when no record names a target.
	names := make(map[string]int)
	for i, v := range o.symbols {
		if v.global && v.section != -3 {
			names[v.name] = i
		}
	}
	for _, v := range indirect {
		index, ok := names[v.target]
		if !ok {
			index = len(o.symbols)
			o.symbols = append(o.symbols, symbol{name: v.target, global: true})
			names[v.target] = index
		}
		o.symbols[v.index].forward = &symbolForward{target: index}
	}
}
