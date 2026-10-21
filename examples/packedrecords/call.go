//go:build cgo

// Package packedrecords demonstrates generated compiler adapters for packed C
// values. Public Go records are logical values, never views of packed storage.
package packedrecords

import (
	"os"
	"path/filepath"
	"runtime"

	dylib "github.com/ramblelabs/dylib-go"
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
	p, err := b.Echo_packed(BindingsRecord0{Tag: -128, Value: 20.5, Tail: 21, Truth: true})
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
