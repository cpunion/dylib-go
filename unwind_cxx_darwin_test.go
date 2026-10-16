//go:build darwin

package dylib

import "testing"

func TestNativeMachOCXXExceptions(t *testing.T) { testNativeCXXExceptions(t) }

func TestNativeMachOCompactCXXExceptions(t *testing.T) { testNativeCXXExceptionsMode(t, true) }

func TestNativeMachOCompactFramelessCXXExceptions(t *testing.T) {
	testNativeCXXExceptionsMode(t, true, "-fomit-frame-pointer")
}

func TestNativeMachOCompactLargeStackCXXExceptions(t *testing.T) {
	testNativeCXXExceptionsMode(t, true, "-fomit-frame-pointer", "-DDYLIB_CXX_LARGE_STACK", "-fno-stack-check")
}

func TestNativeMachOCompactDefaultLargeStackCXXExceptions(t *testing.T) {
	testNativeCXXExceptionsMode(t, true, "-fomit-frame-pointer", "-DDYLIB_CXX_LARGE_STACK")
}
