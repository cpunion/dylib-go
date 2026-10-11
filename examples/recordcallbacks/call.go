//go:build cgo && (linux || darwin || windows)

package recordcallbacks

import (
	"os"
	"path/filepath"
	"runtime"

	dylib "github.com/cpunion/dylib-go"
)

// Call passes a record containing a native C entry by value.
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
	record, err := bindings.Callbacks_factory(0)
	if err != nil {
		return 0, err
	}
	return bindings.Callbacks_apply(record, 20, 22)
}

func load(path string) (*dylib.Session, error) {
	s := dylib.New(dylib.Options{ProcessSymbols: runtime.GOOS != "windows"})
	if runtime.GOOS == "windows" {
		directory := "System32"
		if runtime.GOARCH == "386" {
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
