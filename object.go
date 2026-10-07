// Package dylib loads native object files, archives and shared libraries.
// Parsing and relocation are implemented in Go. Native calls use a small C ABI
// bridge, which can also be compiled by llgo.
package dylib

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

// Info describes a file without loading or executing it. Unsupported lists features
// that prevent direct object linking in this implementation, not parser errors.
type Info struct {
	Name        string       `json:"name"`
	Format      string       `json:"format"`
	Kind        string       `json:"kind"`
	Arch        string       `json:"arch,omitempty"`
	OS          string       `json:"os,omitempty"`
	CPUSubtype  uint32       `json:"cpu_subtype,omitempty"`
	Bits        int          `json:"bits,omitempty"`
	Symbols     []SymbolInfo `json:"symbols,omitempty"`
	Members     []Info       `json:"members,omitempty"`
	Unsupported []string     `json:"unsupported,omitempty"`
}

type SymbolInfo struct {
	Name    string `json:"name"`
	Defined bool   `json:"defined"`
	Weak    bool   `json:"weak,omitempty"`
}

// Inspect reads metadata only; it can inspect foreign targets on any host.
func Inspect(path string) (Info, error) {
	b, err := readFile(path)
	if err != nil {
		return Info{}, err
	}
	f, err := parse(path, b)
	if err != nil {
		return Info{}, err
	}
	return f.info, nil
}

const maxFile = 256 << 20
const maxImage = 64 << 20

func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > maxFile {
		return nil, fmt.Errorf("%s: expected regular file of at most %d bytes", path, maxFile)
	}
	b := make([]byte, st.Size())
	_, err = f.ReadAt(b, 0)
	return b, err
}

type file struct {
	info    Info
	obj     *object
	members []*file
}
type object struct {
	info     Info
	sections []*section // Original section indices (zero reserved).
	symbols  []symbol   // Original symbol table indices.
	relocs   []relocation
	groups   []sectionGroup
}

// ELF COMDAT groups and COFF COMDAT sections share selection and discard rules.
// COFF association is an original section index within the same object.
type sectionGroup struct {
	key       string
	sections  []int
	selection uint8 // COFF NODUPLICATES=1, ANY=2, SAME_SIZE=3, ASSOCIATIVE=5, LARGEST=6.
	parent    int
}
type section struct {
	name                  string
	data                  []byte
	size, align, original uint64
	exec, write           bool
	offset                uint64 // Assigned only in the private image built by Link.
}

// section: 0 undefined, -1 absolute, -2 common, -3 ignored/debug/unsupported.
type symbol struct {
	name               string
	section            int
	value, size, align uint64
	global, weak       bool
}
type relocation struct {
	section  int
	offset   uint64
	typ      uint32
	symbol   int
	addend   int64
	width    int
	pcrel    bool
	local    bool // Mach-O section ordinal instead of symbol-table index.
	implicit bool // ELF REL takes its addend from the relocated word.
	pair     int  // Mach-O subtractor: minuend; -1 otherwise.
}

func parse(name string, b []byte) (*file, error) {
	if len(b) > maxFile {
		return nil, fmt.Errorf("%s: file too large", name)
	}
	var f *file
	var err error
	switch {
	case bytes.HasPrefix(b, []byte("!<arch>\n")):
		f, err = parseArchive(name, b)
	case bytes.HasPrefix(b, []byte("!<thin>\n")):
		err = fmt.Errorf("thin archives are not supported; use a regular archive")
	case bytes.HasPrefix(b, []byte("\x7fELF")):
		f, err = parseELF(name, b)
	case len(b) >= 4 && (binary.LittleEndian.Uint32(b) == 0xfeedfacf || binary.LittleEndian.Uint32(b) == 0xfeedface || binary.BigEndian.Uint32(b) == 0xfeedfacf || binary.BigEndian.Uint32(b) == 0xfeedface):
		f, err = parseMachO(name, b)
	case len(b) >= 4 && (binary.BigEndian.Uint32(b) == 0xcafebabe || binary.BigEndian.Uint32(b) == 0xcafebabf):
		err = fmt.Errorf("universal Mach-O: extract a single target slice first")
	case bytes.HasPrefix(b, []byte("MZ")) || len(b) >= 20 && (binary.LittleEndian.Uint16(b) == 0x8664 || binary.LittleEndian.Uint16(b) == 0xaa64 || binary.LittleEndian.Uint16(b) == 0x14c):
		f, err = parseCOFF(name, b)
	case len(b) >= 4 && binary.LittleEndian.Uint32(b) == 0xffff0000:
		err = fmt.Errorf("COFF import objects/bigobj are unsupported; load the DLL directly")
	case bytes.HasPrefix(b, []byte("BC\xc0\xde")) || bytes.HasPrefix(b, []byte{0xde, 0xc0, 0x17, 0x0b}):
		err = fmt.Errorf("LLVM bitcode: compile to a native object with clang -c first")
	default:
		err = fmt.Errorf("unsupported file format (OMF, Go gc archives, Wasm and raw IR are not native objects)")
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return f, nil
}

func finish(o *object) *file {
	for _, s := range o.symbols {
		if s.global && s.name != "" {
			o.info.Symbols = append(o.info.Symbols, SymbolInfo{s.name, s.section != 0 && s.section != -3, s.weak})
		}
	}
	return &file{info: o.info, obj: o}
}

func (o *object) unsupported(s string) { o.info.Unsupported = append(o.info.Unsupported, s) }

func validSection(s *section) error {
	if s.size > maxImage || s.align > maxImage || s.align != 0 && s.align&(s.align-1) != 0 {
		return fmt.Errorf("section %s: invalid size/alignment", s.name)
	}
	if uint64(len(s.data)) > s.size {
		return fmt.Errorf("section %s: data exceeds size", s.name)
	}
	if s.exec && s.write {
		return fmt.Errorf("section %s requests writable executable memory", s.name)
	}
	return nil
}
