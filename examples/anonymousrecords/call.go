//go:build cgo

// Package anonymousrecords demonstrates compiler-qualified anonymous record typedef
// alignment through C bridges. Public records are ordinary Go values.
package anonymousrecords

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
	value, err := bindings.Echo_high(BindingsRecord0{High_value: 42})
	if err != nil {
		return 0, err
	}
	return value.High_value, nil
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
