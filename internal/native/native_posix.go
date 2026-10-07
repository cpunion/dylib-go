//go:build cgo && (darwin || linux)

package native

/*
#cgo linux LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
static uintptr_t dylib_open(const char *name, char *err, size_t n) {
    void *p = dlopen(name, RTLD_NOW | RTLD_LOCAL);
    if (!p) snprintf(err, n, "%s", dlerror());
    return (uintptr_t)p;
}
static uintptr_t dylib_sym(uintptr_t h, const char *name) {
    return (uintptr_t)dlsym(h ? (void *)h : RTLD_DEFAULT, name);
}
static int dylib_close(uintptr_t h, char *err, size_t n) {
    int rc = dlclose((void *)h);
    if (rc) snprintf(err, n, "%s", dlerror());
    return rc;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func Open(path string) (uintptr, error) {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	var msg [1024]C.char
	h := C.dylib_open(p, &msg[0], 1024)
	if h == 0 {
		return 0, fmt.Errorf("dlopen: %s", C.GoString(&msg[0]))
	}
	return uintptr(h), nil
}
func Lookup(h uintptr, name string) uintptr {
	p := C.CString(name)
	defer C.free(unsafe.Pointer(p))
	return uintptr(C.dylib_sym(C.uintptr_t(h), p))
}
func Close(h uintptr) error {
	var msg [1024]C.char
	if C.dylib_close(C.uintptr_t(h), &msg[0], 1024) != 0 {
		return fmt.Errorf("dlclose: %s", C.GoString(&msg[0]))
	}
	return nil
}
