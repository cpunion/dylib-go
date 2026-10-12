//go:build cgo

// Package bitfieldrecords demonstrates compiler adapters for named C bitfields.
// Public Go records are logical values, never views of native bitfield storage.
package bitfieldrecords

import (
	"os"
	"path/filepath"
	"runtime"

	dylib "github.com/cpunion/dylib-go"
)

func Call(path string) (float64, error) {
	session, err := load(path)
	if err != nil {
		return 0, err
	}
	defer session.Close()
	b, err := NewBindings(session)
	if err != nil {
		return 0, err
	}
	p, err := b.Echo_bits(BindingsRecord0{Flags: 7, Delta: -8, Active: true, Wide: 42, Value: 47.5})
	if err != nil {
		return 0, err
	}
	return p.Value + float64(p.Delta), nil
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
