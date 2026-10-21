//go:build libffi && cgo && (linux || darwin || windows)

package dylib

import "testing"

func TestNativeMSVCEmulatedTLSGoCallbacks(t *testing.T) {
	needMSVCTLS(t)
	TestNativeEmulatedTLSThreadDestructorGoCallbacks(t)
}

func TestNativeMSVCEmulatedTLSConstructorReentry(t *testing.T) {
	needMSVCTLS(t)
	testTLSConstructorReentry(t, false)
}
