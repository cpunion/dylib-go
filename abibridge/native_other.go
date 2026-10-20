//go:build !cgo || !darwin || (!amd64 && !arm64)

package abibridge

import "unsafe"

func nativeAvailable() bool                                        { return false }
func nativeVersion(uintptr) uint32                                 { return 0 }
func nativePrepare(uintptr, []byte) (uintptr, error)               { return 0, ErrUnsupported }
func nativeInvoke(uintptr, uintptr, uintptr, []byte, []byte) error { return ErrUnsupported }
func nativeRelease(uintptr, uintptr)                               {}

func nativeValueType(uintptr, uintptr) (uintptr, ValueLayout, error) {
	return 0, ValueLayout{}, ErrUnsupported
}
func nativeValueNew(uintptr, uintptr, uintptr) (uintptr, error) { return 0, ErrUnsupported }
func nativeValuePointer(uintptr, uintptr) unsafe.Pointer        { return nil }
