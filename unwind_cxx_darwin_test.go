//go:build darwin

package dylib

import "testing"

func TestNativeMachOCXXExceptions(t *testing.T) { testNativeCXXExceptions(t) }
