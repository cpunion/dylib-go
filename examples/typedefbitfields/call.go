//go:build cgo

// Package typedefbitfields demonstrates compiler-qualified aligned typedef
// bitfields through C bridges. Public records are ordinary Go values.
package typedefbitfields

import (
	"os"
	"path/filepath"
	"runtime"

	dylib "github.com/ramblelabs/dylib-go"
)

func Call(path string) (int32, error) {
	session, err := load(path)
	if err != nil {
		return 0, err
	}
	defer session.Close()
	bindings, err := NewBindings(session)
	if err != nil {
		return 0, err
	}
	value, err := bindings.Echo_low(BindingsRecord0{Low_tag: 13, Low_bits: 7, Low_tail: 22})
	if err != nil {
		return 0, err
	}
	return int32(value.Low_tag) + int32(value.Low_bits) + int32(value.Low_tail), nil
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
