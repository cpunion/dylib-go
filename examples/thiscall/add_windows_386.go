//go:build cgo

// Package thiscall demonstrates generated Windows x86 C-convention bindings.
package thiscall

import dylib "github.com/cpunion/dylib-go"

// Add uses an explicit native receiver from the MSVC-targeted fixture.
// The generated C bridge supplies ECX.
// The fixture owns receiver storage for the lifetime of the loaded image.
func Add(session *dylib.Session) (int32, error) {
	bindings, err := NewBindings(session)
	if err != nil {
		return 0, err
	}
	receiver, err := bindings.This_state()
	if err != nil {
		return 0, err
	}
	return bindings.This_add(receiver, 22)
}
