//go:build !libffi || !cgo

package abi

func prepareCallback(*callbackState) (callbackBackend, error) { return nil, ErrUnavailable }
