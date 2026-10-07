//go:build !cgo || (!darwin && !linux && !windows)

package examplecall

import "fmt"

func Int32(uintptr, int32, int32) (int32, error) {
	return 0, fmt.Errorf("example calls require cgo or llgo on a supported host")
}
