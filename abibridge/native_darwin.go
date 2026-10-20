//go:build cgo && darwin && (amd64 || arm64)

package abibridge

/*
#include <stdint.h>
static uint32_t bridge_version(uintptr_t entry) {
    return ((uint32_t (*)(void))entry)();
}
static uintptr_t bridge_prepare(uintptr_t entry, const unsigned char *data, uint32_t size, char *error, uint32_t capacity) {
    return (uintptr_t)((void *(*)(const unsigned char *, uint32_t, char *, uint32_t))entry)(data, size, error, capacity);
}
static int32_t bridge_invoke(uintptr_t entry, uintptr_t plan, uintptr_t function,
    const unsigned char *args, uint32_t args_size, unsigned char *result, uint32_t result_size,
    char *error, uint32_t capacity) {
    return ((int32_t (*)(void *, const void *, const unsigned char *, uint32_t, unsigned char *, uint32_t, char *, uint32_t))entry)
        ((void *)plan, (const void *)function, args, args_size, result, result_size, error, capacity);
}
static void bridge_release(uintptr_t entry, uintptr_t plan) {
    ((void (*)(void *))entry)((void *)plan);
}
*/
import "C"

import (
	"bytes"
	"fmt"
	"runtime"
	"unsafe"
)

func nativeAvailable() bool              { return true }
func nativeVersion(entry uintptr) uint32 { return uint32(C.bridge_version(C.uintptr_t(entry))) }

func bytePointer(data []byte) *C.uchar {
	if len(data) == 0 {
		return nil
	}
	return (*C.uchar)(unsafe.Pointer(&data[0]))
}

func nativeFailure(data []byte) error {
	if end := bytes.IndexByte(data, 0); end >= 0 {
		data = data[:end]
	}
	if len(data) == 0 {
		return fmt.Errorf("abibridge: native operation failed")
	}
	return fmt.Errorf("abibridge: %s", data)
}

func nativePrepare(entry uintptr, wire []byte) (uintptr, error) {
	var message [1024]byte
	plan := uintptr(C.bridge_prepare(C.uintptr_t(entry), bytePointer(wire), C.uint32_t(len(wire)), (*C.char)(unsafe.Pointer(&message[0])), C.uint32_t(len(message))))
	runtime.KeepAlive(wire)
	if plan == 0 {
		return 0, nativeFailure(message[:])
	}
	return plan, nil
}

func nativeInvoke(entry, plan, function uintptr, args, result []byte) error {
	var message [1024]byte
	status := C.bridge_invoke(C.uintptr_t(entry), C.uintptr_t(plan), C.uintptr_t(function), bytePointer(args), C.uint32_t(len(args)), bytePointer(result), C.uint32_t(len(result)), (*C.char)(unsafe.Pointer(&message[0])), C.uint32_t(len(message)))
	runtime.KeepAlive(args)
	runtime.KeepAlive(result)
	if status != 0 {
		return nativeFailure(message[:])
	}
	return nil
}

func nativeRelease(entry, plan uintptr) {
	C.bridge_release(C.uintptr_t(entry), C.uintptr_t(plan))
}
