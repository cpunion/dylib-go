//go:build cgo

// Package overalignedbits demonstrates compiler-validated C bindings for exports
// with pragma packing and over-aligned bitfield units. Public records are Go values.
package overalignedbits

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
	bindings, err := NewBindings(session)
	if err != nil {
		return 0, err
	}
	value, err := bindings.Echo_bits(BindingsRecord1{Tag: -128, Low: 7, High: -15, Truth: true, Wide: 0xffffffffff, Real: 20.5, Count: 18})
	if err != nil {
		return 0, err
	}
	return value.Real + float64(value.Count), nil
}

func load(path string) (*dylib.Session, error) {
	session := dylib.New(dylib.Options{ProcessSymbols: runtime.GOOS != "windows"})
	if runtime.GOOS == "windows" {
		directory := "System32"
		if runtime.GOARCH == "386" && (os.Getenv("PROCESSOR_ARCHITEW6432") != "" || os.Getenv("PROCESSOR_ARCHITECTURE") == "AMD64") {
			directory = "SysWOW64"
		}
		if err := session.Load(filepath.Join(os.Getenv("SystemRoot"), directory, "msvcrt.dll")); err != nil {
			session.Close()
			return nil, err
		}
	}
	if err := session.Load(path); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}
