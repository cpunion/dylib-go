//go:build !libffi || !cgo

package abi

func prepare(Signature) (callBackend, error)             { return nil, ErrUnavailable }
func Available() bool                                    { return false }
func layoutOf(TypeDesc, Convention) (Layout, error)      { return Layout{}, ErrUnavailable }
func prepareValue(TypeDesc, Value) (valueBackend, error) { return nil, ErrUnavailable }
