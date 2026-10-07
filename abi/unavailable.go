//go:build !libffi || !cgo

package abi

func invoke(uintptr, Signature, []Value) (Value, error) { return Value{}, ErrUnavailable }
func Available() bool                                   { return false }
