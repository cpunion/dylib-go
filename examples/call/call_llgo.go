//go:build llgo && cgo && (darwin || linux || windows)

package call

import "unsafe"

// This adapter knows the exact C signature. The gc compiler cannot use this
// function-pointer representation; it uses the separate cgo adapter instead.
//
//llgo:type C
type int32Func func(int32, int32) int32

func Int32(address uintptr, a, b int32) (int32, error) {
	f := *(*int32Func)(unsafe.Pointer(&address))
	return f(a, b), nil
}
