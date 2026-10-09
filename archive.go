package dylib

import (
	"bytes"
	"fmt"

	"github.com/cpunion/dylib-go/internal/ar"
)

type archiveEntry = ar.Entry

func archiveEntries(b []byte, thin bool) ([]archiveEntry, error) {
	return ar.Decode(b, thin, maxFile, 0)
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
		child, err := archiveObject(name+"("+entry.Name+")", entry.Data)
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
