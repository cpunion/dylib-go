package dylib

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// archiveEntry offsets identify headers, including GNU thin references to
// members of external regular archives. Thin object contents are not inline.
type archiveEntry struct {
	name           string
	data           []byte
	offset, origin uint64
}

// Decode and validate the complete container before opening external members.
func archiveEntries(b []byte, thin bool) ([]archiveEntry, error) {
	var names []byte
	var entries []archiveEntry
	for pos := 8; pos < len(b); {
		if len(b)-pos < 60 || string(b[pos+58:pos+60]) != "`\n" {
			return nil, fmt.Errorf("bad archive header at %d", pos)
		}
		offset := uint64(pos)
		h := b[pos : pos+60]
		n, err := strconv.ParseUint(strings.TrimSpace(string(h[48:58])), 10, 64)
		if err != nil || n > maxFile {
			return nil, fmt.Errorf("invalid archive member size at %d", pos)
		}
		mn := strings.TrimSpace(string(h[:16]))
		stored := !thin || mn == "//" || mn == "/" || mn == "/SYM64/" || strings.HasPrefix(mn, "__.SYMDEF")
		pos += 60
		var member []byte
		if stored {
			if n > uint64(len(b)-pos) {
				return nil, fmt.Errorf("invalid archive member size at %d", offset)
			}
			member = b[pos : pos+int(n)]
			pos += int(n)
			if pos&1 != 0 {
				if pos >= len(b) {
					return nil, fmt.Errorf("missing archive padding")
				}
				pos++
			}
		}
		var origin uint64
		switch {
		case mn == "//":
			names = member
			continue
		case mn == "/" || mn == "/SYM64/" || strings.HasPrefix(mn, "__.SYMDEF"):
			continue
		case strings.HasPrefix(mn, "#1/"):
			if thin {
				return nil, fmt.Errorf("BSD extended names in thin archives are unsupported")
			}
			l, e := strconv.Atoi(mn[3:])
			if e != nil || l < 0 || l > len(member) {
				return nil, fmt.Errorf("invalid BSD archive name")
			}
			mn = strings.TrimRight(string(member[:l]), "\x00")
			member = member[l:]
			if strings.HasPrefix(mn, "__.SYMDEF") {
				continue
			}
		case strings.HasPrefix(mn, "/"):
			reference := mn[1:]
			if thin && strings.Contains(reference, ":") {
				parts := strings.Split(reference, ":")
				if len(parts) != 2 {
					return nil, fmt.Errorf("invalid thin archive member offset")
				}
				origin, err = strconv.ParseUint(parts[1], 10, 64)
				if err != nil || origin < 8 || origin&1 != 0 {
					return nil, fmt.Errorf("invalid thin archive member offset")
				}
				reference = parts[0]
			}
			o, e := strconv.Atoi(reference)
			if e != nil || o < 0 || o >= len(names) {
				return nil, fmt.Errorf("invalid archive name offset")
			}
			// GNU long names end with /\n; COFF long names end with NUL.
			// Both use the // member and /decimal-offset references.
			end := bytes.IndexAny(names[o:], "\n\x00")
			if end < 0 {
				return nil, fmt.Errorf("unterminated archive name")
			}
			mn = string(names[o : o+end])
			if names[o+end] == '\n' {
				mn = strings.TrimSuffix(mn, "/")
			}
		default:
			mn = strings.TrimSuffix(mn, "/")
		}
		if mn == "" {
			return nil, fmt.Errorf("empty archive member name")
		}
		if strings.IndexByte(mn, 0) >= 0 {
			return nil, fmt.Errorf("NUL in archive member name")
		}
		entries = append(entries, archiveEntry{name: mn, data: member, offset: offset, origin: origin})
	}
	return entries, nil
}

// parseArchive handles GNU/SysV, COFF and BSD extended names. Index members
// are skipped: symbols are indexed from the objects rather than ranlib tables.
func parseArchive(name string, b []byte) (*file, error) {
	entries, err := archiveEntries(b, false)
	if err != nil {
		return nil, err
	}
	f := &file{info: Info{Name: name, Format: "ar", Kind: "archive"}}
	for _, entry := range entries {
		child, err := archiveObject(name+"("+entry.name+")", entry.data)
		if err != nil {
			return nil, err
		}
		f.members = append(f.members, child)
	}
	finishArchive(f)
	return f, nil
}

func archiveObject(name string, b []byte) (*file, error) {
	if bytes.HasPrefix(b, []byte("!<arch>\n")) || bytes.HasPrefix(b, []byte("!<thin>\n")) {
		return nil, fmt.Errorf("%s: nested archives are unsupported", name)
	}
	child, err := parse(name, b)
	if err != nil {
		return nil, err
	}
	if child.info.Kind != "object" {
		return nil, fmt.Errorf("%s: archive members must be relocatable objects", name)
	}
	return child, nil
}

func finishArchive(f *file) {
	objects := make([]*object, len(f.members))
	for i, child := range f.members {
		objects[i] = child.obj
	}
	imports := newCOFFImportGraph(objects)
	f.info.Members = nil
	for _, child := range f.members {
		if imported, ok := imports.convert(child.obj); ok {
			imported.directory = child.obj.directory
			child.obj, child.info = imported, finish(imported).info
		}
		f.info.Members = append(f.info.Members, child.info)
	}
}
