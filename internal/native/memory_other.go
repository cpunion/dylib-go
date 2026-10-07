//go:build !darwin && !linux && !windows

package native

import "fmt"

func Alloc(int) ([]byte, error) {
	return nil, fmt.Errorf("executable memory unavailable on this host")
}
func Free([]byte) error { return nil }
func Protect([]byte, bool, bool) error {
	return fmt.Errorf("executable memory unavailable on this host")
}
