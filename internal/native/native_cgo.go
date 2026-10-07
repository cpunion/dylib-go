//go:build cgo && (darwin || linux || windows)

package native

/*
#include <stdint.h>
static void dylib_clear_cache(void *p, uintptr_t n) {
#if defined(__GNUC__) || defined(__clang__)
    __builtin___clear_cache((char *)p, (char *)p + n);
#endif
}
*/
import "C"

import "unsafe"

func ClearCache(b []byte) {
	if len(b) > 0 {
		C.dylib_clear_cache(unsafe.Pointer(&b[0]), C.uintptr_t(len(b)))
	}
}
func Available() bool { return true }
