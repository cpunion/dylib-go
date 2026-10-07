//go:build cgo && !llgo && (darwin || linux || windows)

// Package call demonstrates a known C signature for the CLI and compiler
// probes. Applications supply their own signatures; this is an example adapter.
package call

/*
#include <stdint.h>
static int32_t dylib_example_call(uintptr_t p, int32_t a, int32_t b) {
    return ((int32_t (*)(int32_t, int32_t))p)(a, b);
}
*/
import "C"

func Int32(address uintptr, a, b int32) (int32, error) {
	return int32(C.dylib_example_call(C.uintptr_t(address), C.int32_t(a), C.int32_t(b))), nil
}
