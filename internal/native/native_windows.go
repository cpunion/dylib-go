//go:build windows

package native

import (
	"fmt"
	"syscall"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var virtualAlloc = kernel32.NewProc("VirtualAlloc")
var virtualProtect = kernel32.NewProc("VirtualProtect")
var virtualFree = kernel32.NewProc("VirtualFree")
var flushInstructionCache = kernel32.NewProc("FlushInstructionCache")

func Alloc(n int) ([]byte, error) {
	p, _, e := virtualAlloc.Call(0, uintptr(n), 0x3000, 0x04)
	if p == 0 {
		return nil, e
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(p)), n), nil
}
func Free(b []byte) error {
	r, _, e := virtualFree.Call(uintptr(unsafe.Pointer(&b[0])), 0, 0x8000)
	if r == 0 {
		return e
	}
	return nil
}
func Protect(b []byte, exec, write bool) error {
	p := uintptr(0x02)
	if write {
		p = 0x04
	}
	if exec {
		p = 0x20
	}
	var old uint32
	r, _, e := virtualProtect.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), p, uintptr(unsafe.Pointer(&old)))
	if r == 0 {
		return e
	}
	if exec {
		r, _, e = flushInstructionCache.Call(^uintptr(0), uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
		if r == 0 {
			return e
		}
	}
	return nil
}
func Open(path string) (uintptr, error) {
	h, e := syscall.LoadLibrary(path)
	return uintptr(h), e
}
func Lookup(h uintptr, name string) uintptr {
	if h == 0 {
		return 0
	}
	p, _ := syscall.GetProcAddress(syscall.Handle(h), name)
	return p
}
func Close(h uintptr) error {
	if h == 0 {
		return fmt.Errorf("invalid library handle")
	}
	return syscall.FreeLibrary(syscall.Handle(h))
}
