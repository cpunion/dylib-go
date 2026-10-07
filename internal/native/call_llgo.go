//go:build llgo && cgo && (darwin || linux || windows)

package native

import "unsafe"

// llgo represents a C function value as a native function pointer and emits
// an indirect C-ABI call. This conversion is not valid for the gc compiler.
//
//llgo:type C
type nativeInt32Func func(int32, int32) int32

func CallInt32(p uintptr, a, b int32) int32 {
	f := *(*nativeInt32Func)(unsafe.Pointer(&p))
	return f(a, b)
}
