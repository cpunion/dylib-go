//go:build !cgo || !darwin || (!amd64 && !arm64)

package abibridge

func nativeAvailable() bool                                        { return false }
func nativeVersion(uintptr) uint32                                 { return 0 }
func nativePrepare(uintptr, []byte) (uintptr, error)               { return 0, ErrUnsupported }
func nativeInvoke(uintptr, uintptr, uintptr, []byte, []byte) error { return ErrUnsupported }
func nativeRelease(uintptr, uintptr)                               {}
