//go:build darwin || linux

package native

import "syscall"

func Alloc(n int) ([]byte, error) {
	return syscall.Mmap(-1, 0, n, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
}
func Free(b []byte) error { return syscall.Munmap(b) }
func Protect(b []byte, exec, write bool) error {
	p := syscall.PROT_READ
	if exec {
		p |= syscall.PROT_EXEC
	}
	if write {
		p |= syscall.PROT_WRITE
	}
	return syscall.Mprotect(b, p)
}
