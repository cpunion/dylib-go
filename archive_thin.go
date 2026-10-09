package dylib

import (
	"context"
	"path/filepath"

	"github.com/cpunion/dylib-go/internal/ar"
)

func parseThinArchive(name string, b []byte) (*file, error) {
	entries, err := ar.Snapshot(context.Background(), name, b, maxFile, 0, readFileLimit)
	if err != nil {
		return nil, err
	}
	f := &file{info: Info{Name: name, Format: "ar", Kind: "archive", Thin: true}}
	for _, entry := range entries {
		child, err := archiveObject(name+"("+entry.DisplayName+")", entry.Data)
		if err != nil {
			return nil, err
		}
		child.obj.directory = filepath.Dir(entry.Source)
		f.members = append(f.members, child)
	}
	finishArchive(f)
	return f, nil
}
