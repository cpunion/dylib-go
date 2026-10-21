//go:build cgo && (linux || darwin || windows)

// Package foreignabi demonstrates generated Microsoft x64 and System V calls.
package foreignabi

import dylib "github.com/ramblelabs/dylib-go"

// AddBoth calls the same logical C signature with two native conventions.
func AddBoth(session *dylib.Session) (microsoft, systemV int32, err error) {
	b, err := NewBindings(session)
	if err != nil {
		return 0, 0, err
	}
	microsoft, err = b.Ms_add(20, 22)
	if err != nil {
		return 0, 0, err
	}
	systemV, err = b.Sysv_add(20, 22)
	return
}
