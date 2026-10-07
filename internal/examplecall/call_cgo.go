//go:build cgo && !llgo && (darwin || linux || windows)

// Package examplecall supplies one known C signature for the CLI and compiler
// probes. It is an example adapter, not part of the loader's public API.
package examplecall

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
