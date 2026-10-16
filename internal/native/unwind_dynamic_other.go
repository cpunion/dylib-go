//go:build !darwin || !cgo

package native

import "fmt"

func RegisterMachODWARF(uintptr, uintptr, []uintptr) (uintptr, error) {
	return 0, fmt.Errorf("dynamic Mach-O unwind lookup requires native macOS")
}

func UnregisterMachODWARF(uintptr, uintptr) error {
	return fmt.Errorf("dynamic Mach-O unwind lookup requires native macOS")
}
