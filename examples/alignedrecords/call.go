//go:build cgo

// Package alignedrecords demonstrates compiler adapters for explicitly aligned
// C values. Go records carry logical fields, not a view of native storage.
package alignedrecords

import (
	"os"
	"path/filepath"
	"runtime"

	dylib "github.com/ramblelabs/dylib-go"
)

func Call(path string) (float64, error) {
	s, err := load(path)
	if err != nil {
		return 0, err
	}
	defer s.Close()
	b, err := NewBindings(s)
	if err != nil {
		return 0, err
	}
	p, err := b.Echo_aligned(BindingsRecord0{Tag: -128, Value: 18.5, Tail: 20})
	if err != nil {
		return 0, err
	}
	return p.Value + float64(p.Tail), nil
}

func load(path string) (*dylib.Session, error) {
	s := dylib.New(dylib.Options{ProcessSymbols: runtime.GOOS != "windows"})
	if runtime.GOOS == "windows" {
		directory := "System32"
		if runtime.GOARCH == "386" && (os.Getenv("PROCESSOR_ARCHITEW6432") != "" || os.Getenv("PROCESSOR_ARCHITECTURE") == "AMD64") {
			directory = "SysWOW64"
		}
		if err := s.Load(filepath.Join(os.Getenv("SystemRoot"), directory, "msvcrt.dll")); err != nil {
			s.Close()
			return nil, err
		}
	}
	if err := s.Load(path); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
