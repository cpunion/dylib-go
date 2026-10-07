//go:build !libffi || !cgo

package abi

func invoke(uintptr, Signature, []Value) (uint64, error) { return 0, ErrUnavailable }
func Available() bool                                    { return false }
