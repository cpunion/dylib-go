//go:build (!cgo && !windows) || (!darwin && !linux && !windows)

package native

import "fmt"

func Open(string) (uintptr, error) {
	return 0, fmt.Errorf("shared library loading unavailable in this build")
}
func Lookup(uintptr, string) uintptr { return 0 }
func Close(uintptr) error            { return nil }
