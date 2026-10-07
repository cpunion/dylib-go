//go:build cgo && !llgo && (darwin || linux || windows)

package native

/*
#include <stdint.h>
static int32_t dylib_call_i32(uintptr_t p, int32_t a, int32_t b) {
    return ((int32_t (*)(int32_t, int32_t))p)(a, b);
}
*/
import "C"

func CallInt32(p uintptr, a, b int32) int32 {
	return int32(C.dylib_call_i32(C.uintptr_t(p), C.int32_t(a), C.int32_t(b)))
}
