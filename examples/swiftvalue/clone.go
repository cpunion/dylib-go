// Package swiftvalue demonstrates owned Swift storage with copy/destroy witnesses.
package swiftvalue

import "github.com/cpunion/dylib-go/abibridge"

// CloneAndUse initializes a value, copies it, and uses the copy after closing
// the original. Adapters receive borrowed native storage, never ownership bytes.
// The caller supplies compiler-matched adapters and keeps the Runtime open.
func CloneAndUse(kind *abibridge.ValueType, initialize, use func(uintptr) error) error {
	value, err := kind.New(initialize)
	if err != nil {
		return err
	}
	defer value.Close()
	copy, err := value.Clone()
	if err != nil {
		return err
	}
	defer copy.Close()
	if err := value.Close(); err != nil {
		return err
	}
	return copy.WithAddress(use)
}
