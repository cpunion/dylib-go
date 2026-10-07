//go:build !cgo || (!darwin && !linux && !windows)

package native

func CallInt32(uintptr, int32, int32) int32 {
	panic("native calls require cgo or llgo on a supported host")
}
func ClearCache([]byte) {}
func Available() bool   { return false }
