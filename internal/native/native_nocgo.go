//go:build !cgo || (!darwin && !linux && !windows)

package native

func ClearCache([]byte) {}
func Available() bool   { return false }
