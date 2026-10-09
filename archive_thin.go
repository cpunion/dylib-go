package dylib

import (
	"bytes"
	"fmt"
	"path/filepath"
)

// External paths are read once, so their proxy references see the same snapshot.
// Bound both file reads and decoded object bytes: repeated references must not
// turn a small thin container into unbounded parsing/allocation work.
type thinSource struct {
	data    []byte
	entries map[uint64]archiveEntry
}

type thinReader struct {
	remaining int64
	sources   map[string]*thinSource
}

func (r *thinReader) read(path string) (*thinSource, error) {
	if source := r.sources[path]; source != nil {
		return source, nil
	}
	b, err := readFileLimit(path, r.remaining)
	if err != nil {
		return nil, fmt.Errorf("thin archive external member: %w", err)
	}
	r.remaining -= int64(len(b))
	source := &thinSource{data: b}
	r.sources[path] = source
	return source, nil
}

func (s *thinSource) member(offset uint64) (archiveEntry, error) {
	if s.entries == nil {
		if !bytes.HasPrefix(s.data, []byte("!<arch>\n")) {
			return archiveEntry{}, fmt.Errorf("thin member offset requires an external regular archive")
		}
		entries, err := archiveEntries(s.data, false)
		if err != nil {
			return archiveEntry{}, err
		}
		s.entries = make(map[uint64]archiveEntry, len(entries))
		for _, entry := range entries {
			s.entries[entry.Offset] = entry
		}
	}
	entry, ok := s.entries[offset]
	if !ok {
		return archiveEntry{}, fmt.Errorf("thin archive offset %d is not an object member header", offset)
	}
	return entry, nil
}

func parseThinArchive(name string, b []byte) (*file, error) {
	entries, err := archiveEntries(b, true)
	if err != nil {
		return nil, err
	}
	f := &file{info: Info{Name: name, Format: "ar", Kind: "archive", Thin: true}}
	r := thinReader{remaining: maxFile - int64(len(b)), sources: make(map[string]*thinSource)}
	remainingObjects := maxFile
	for _, entry := range entries {
		path := filepath.FromSlash(entry.Name)
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(name), path)
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		source, err := r.read(path)
		if err != nil {
			return nil, err
		}
		member, memberName := source.data, name+"("+entry.Name+")"
		if entry.Origin != 0 {
			inner, err := source.member(entry.Origin)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			member = inner.Data
			memberName = fmt.Sprintf("%s(%s:%d(%s))", name, entry.Name, entry.Origin, inner.Name)
		}
		if len(member) > remainingObjects {
			return nil, fmt.Errorf("thin archive decoded object bytes exceed %d", maxFile)
		}
		remainingObjects -= len(member)
		child, err := archiveObject(memberName, member)
		if err != nil {
			return nil, err
		}
		child.obj.directory = filepath.Dir(path)
		f.members = append(f.members, child)
	}
	finishArchive(f)
	return f, nil
}
