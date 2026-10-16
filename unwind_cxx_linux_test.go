//go:build linux

package dylib

import "testing"

func TestNativeELFCXXExceptions(t *testing.T) { testNativeCXXExceptions(t) }
